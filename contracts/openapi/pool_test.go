package openapi

import (
	"net/http"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/testhttp"
)

func TestPublicOpenAPIPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	spec := fixtureSpec(
		`{"/value":{"get":{"operationId":"value","responses":{"200":{"description":"value","content":{"application/json":{"schema":{"type":"object"}}}}}}}}`,
		`{}`,
	)
	pool := testhttp.NewPool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/spec.json" {
			_, _ = w.Write([]byte(spec))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	tools, cleanup, err := ParseURLWithCleanup(
		t.Context(),
		pool.Server.URL+"/spec.json",
		Options{AllowPrivateIPs: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	// Act.
	for range 20 {
		if err = tools[0].Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(toolsy.Chunk) error { return nil },
		); err != nil {
			t.Fatal(err)
		}
	}
	// Assert: discovery and all operations share one connection.
	pool.AssertReusedAndClosed(t, cleanup)
}
