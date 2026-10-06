package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
func TestPublicToolRejectsRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, sameOrigin := range []bool{false, true} {
			t.Run(strconv.Itoa(status)+"/same="+strconv.FormatBool(sameOrigin), func(t *testing.T) {
				// Arrange.
				var targets, sources atomic.Int32
				targetHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					targets.Add(1)
					_, _ = w.Write([]byte(`{"data":{"scalar":1}}`))
				})
				target := httptest.NewServer(targetHandler)
				t.Cleanup(target.Close)
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/target" {
						targetHandler(w, r)
						return
					}
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						return
					}
					if strings.Contains(string(body), "__schema") {
						_ = json.NewEncoder(w).Encode(introResponse{Data: &introData{
							Schema: introSchema{QueryType: &introTypeName{Name: "Query"}, Types: fixtureTypes()},
						}})
						return
					}
					sources.Add(1)
					if r.Header.Get("Authorization") != "Bearer origin-secret" {
						t.Error("missing original credentials")
					}
					location := target.URL + "/target"
					if sameOrigin {
						location = "/target"
					}
					http.Redirect(w, r, location, status)
				}))
				t.Cleanup(source.Close)
				tools, err := Introspect(t.Context(), source.URL, Options{
					AllowPrivateIPs: true, Operations: []string{"query"},
					Selections: map[string][]Selection{"query.item": {{Name: "name"}}},
				})
				if err != nil {
					t.Fatal(err)
				}
				var selected toolsy.Tool
				for _, tool := range tools {
					if strings.Contains(tool.Manifest().Name, "scalar") {
						selected = tool
					}
				}
				if selected == nil {
					t.Fatal("missing scalar tool")
				}
				delivered := 0
				// Act.
				err = selected.Execute(
					t.Context(),
					toolsy.NewRunEnv(nil, toolsy.WithCredentials(redirectCredentials{})),
					toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
					func(toolsy.Chunk) error { delivered++; return nil },
				)
				// Assert.

				if err != nil {
					var refused *httptool.RedirectError
					te, ok := toolsy.AsToolError(err)
					if !errors.As(err, &refused) || !ok || te.Code != toolsy.CodeRemoteExecution ||
						te.Retryable || toolsy.ClientCorrectable(te.Code) {
						t.Fatalf("incorrect redirect classification: %v", err)
					}
				}
				if err == nil || sources.Load() != 1 || targets.Load() != 0 || delivered != 0 {
					t.Fatalf(
						"err=%v sources=%d targets=%d delivered=%d",
						err,
						sources.Load(),
						targets.Load(),
						delivered,
					)
				}
			})
		}
	}
}

func TestPublicIntrospectionRejectsRedirect(t *testing.T) {
	// Arrange.
	var targets atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targets.Add(1)
		_ = json.NewEncoder(w).Encode(introResponse{Data: &introData{
			Schema: introSchema{QueryType: &introTypeName{Name: "Query"}, Types: fixtureTypes()},
		}})
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)
	// Act.
	tools, err := Introspect(t.Context(), source.URL, Options{
		AllowPrivateIPs: true, IntrospectionAuthHeader: "Bearer discovery-secret",
		Selections: map[string][]Selection{"query.item": {{Name: "name"}}},
	})
	// Assert.
	if err == nil || len(tools) != 0 || targets.Load() != 0 {
		t.Fatalf("err=%v tools=%d targets=%d", err, len(tools), targets.Load())
	}
}
