package httptool

import (
	"net/http"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/testhttp"
)

func TestPublicHTTPPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	pool := testhttp.NewPool(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }),
	)
	tools, cleanup, err := AsToolsWithCleanup(WithAllowedDomains([]string{"127.0.0.1"}), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	tool := tools[0]
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
