package web

import (
	"net/http"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/testhttp"
)

func TestPublicHTTPPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	pool := testhttp.NewPool(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Hello world.</p></body></html>"))
	}))
	tools, cleanup, err := AsToolsWithCleanup(&mockSearchProvider{}, WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	tool := tools[1]
	// Act.
	for range 20 {
		input := toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + pool.Server.URL + `/file.csv"}`)}
		if err = tool.Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			input,
			func(toolsy.Chunk) error { return nil },
		); err != nil {
			t.Fatal(err)
		}
	}
	// Assert.
	pool.AssertReusedAndClosed(t, cleanup)
}
