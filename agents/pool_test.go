package agents

import (
	"net/http"
	"testing"

	"github.com/skosovsky/toolsy/internal/testhttp"
)

func TestPublicAgentPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	pool := testhttp.NewPool(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)
	client := mustClient(t, pool.Server.URL, WithAllowPrivateIPs(true))
	// Act.
	for range 20 {
		if err := client.CancelTask(t.Context(), "task", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Assert.
	pool.AssertReusedAndClosed(t, client.CloseIdleConnections)
}
