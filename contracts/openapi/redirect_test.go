package openapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

type redirectCredentials struct{}

func (redirectCredentials) GetAuth(context.Context, string) (string, error) {
	return "Bearer origin-secret", nil
}

//nolint:gocognit // Integrated wire matrix checks independent origin, method and status combinations.
func TestPublicToolRedirectContract(t *testing.T) {
	for _, method := range []string{"get", "post"} {
		for _, status := range []int{301, 302, 303, 307, 308} {
			for _, sameOrigin := range []bool{false, true} {
				t.Run(method+"/"+strconv.Itoa(status)+"/same="+strconv.FormatBool(sameOrigin), func(t *testing.T) {
					// Arrange.
					var targets, sources atomic.Int32
					targetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						targets.Add(1)
						if r.Header.Get("Authorization") != "Bearer origin-secret" {
							t.Error("same-origin read lost credentials")
						}
						w.WriteHeader(http.StatusNoContent)
					})
					target := httptest.NewServer(targetHandler)
					t.Cleanup(target.Close)
					spec := fixtureSpec(`{"/call":{"`+method+`":{"responses":{"204":{"description":"ok"}}}}}`, `{}`)
					source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/spec.json":
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(spec))
						case "/target":
							targetHandler(w, r)
						default:
							sources.Add(1)
							if r.Header.Get("Authorization") != "Bearer origin-secret" {
								t.Error("missing original credentials")
							}
							location := target.URL + "/target"
							if sameOrigin {
								location = "/target"
							}
							http.Redirect(w, r, location, status)
						}
					}))
					t.Cleanup(source.Close)
					tools, err := ParseURL(t.Context(), source.URL+"/spec.json", Options{AllowPrivateIPs: true})
					if err != nil || len(tools) != 1 {
						t.Fatalf("discovery: tools=%d err=%v", len(tools), err)
					}
					delivered := 0
					// Act.
					err = tools[0].Execute(
						t.Context(),
						toolsy.NewRunEnv(nil, toolsy.WithCredentials(redirectCredentials{})),
						toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
						func(toolsy.Chunk) error { delivered++; return nil },
					)
					// Assert.
					allowed := method == "get" && sameOrigin
					if (err == nil) != allowed || sources.Load() != 1 {
						t.Fatalf("allowed=%v err=%v sources=%d", allowed, err, sources.Load())
					}

					if err != nil {
						var refused *httptool.RedirectError
						te, ok := toolsy.AsToolError(err)
						if !errors.As(err, &refused) || !ok || te.Code != toolsy.CodeRemoteExecution ||
							te.Retryable || toolsy.ClientCorrectable(te.Code) {
							t.Fatalf("incorrect redirect classification: %v", err)
						}
					}
					want := int32(0)
					if allowed {
						want = 1
					}
					if targets.Load() != want || delivered != int(want) {
						t.Fatalf("targets=%d delivered=%d want=%d", targets.Load(), delivered, want)
					}
				})
			}
		}
	}
}
