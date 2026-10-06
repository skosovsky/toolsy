package document

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
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("a,b\n1,2\n")) }),
	)
	tools, cleanup, err := AsToolWithCleanup(WithAllowRemote(true), WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	tool := tools
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
