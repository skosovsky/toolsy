package graphql

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/testhttp"
)

func TestPublicGraphQLPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	pool := testhttp.NewPool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(input.Query, "__schema") {
			_ = json.NewEncoder(w).
				Encode(introResponse{Data: &introData{Schema: introSchema{QueryType: &introTypeName{Name: "Query"}, Types: fixtureTypes()}}})
			return
		}
		_, _ = w.Write([]byte(`{"data":{"color":"RED"}}`))
	}))
	tools, cleanup, err := IntrospectWithCleanup(
		t.Context(),
		pool.Server.URL,
		Options{AllowPrivateIPs: true, Selections: map[string][]Selection{"query.item": {{Name: "name"}}}},
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
	// Assert.
	pool.AssertReusedAndClosed(t, cleanup)
}
