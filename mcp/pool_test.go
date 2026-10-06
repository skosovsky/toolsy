package mcp

import (
	"net/http"
	"testing"
	"time"

	"github.com/skosovsky/toolsy/internal/testhttp"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestPublicMCPPoolReuseAndCleanup(t *testing.T) {
	// Arrange.
	pool := testhttp.NewPool(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCEnvelope(t, r)
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, envelope.ID, completeDiscovery(ServerCapabilities{}))
	}))
	transport := NewStreamableHTTPTransport(pool.Server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	t.Cleanup(func() { _ = transport.Close() })
	if err := transport.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Act.
	for range 20 {
		if _, err := httpMigrationRequest(transport); err != nil {
			t.Fatal(err)
		}
	}
	// Assert.
	pool.AssertReusedAndClosed(t, func() {
		if err := transport.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPHTTPSettingsFailBeforeDispatch(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport(
		"http://127.0.0.1:1",
		WithStreamableHTTPSettings(httptool.ClientSettings{Timeout: -time.Second}),
	)
	t.Cleanup(func() { _ = transport.Close() })
	// Act.
	err := transport.Start(t.Context())
	// Assert.
	if err == nil || transport.started || transport.client != nil {
		t.Fatalf("err=%v started=%v client=%v", err, transport.started, transport.client)
	}
}
