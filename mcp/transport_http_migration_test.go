package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func migrationDiscoveryParams() DiscoverParams {
	return DiscoverParams{Meta: &RequestMeta{ProtocolVersion: ProtocolVersion,
		ClientInfo: &Implementation{Name: "migration-test", Version: "test"}}}
}

func TestMigratedSSEUsesNoGETOrEndpointDiscovery(t *testing.T) {
	// Arrange: legacy endpoint event must not cause a second endpoint request.
	var posts, others atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			others.Add(1)
		}
		posts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(t.Context()))
	t.Cleanup(func() { _ = transport.Close() })
	require.Zero(t, posts.Load())
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.Zero(t, posts.Load())
	// Act.
	require.NoError(t, pending.Deliver())
	_, awaitErr := pending.Await(t.Context())
	// Assert: no GET, endpoint switch or compatibility recovery.
	require.Error(t, awaitErr)
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, others.Load())
}

func TestMigratedSSEPrivateEndpointFailsBeforeTraffic(t *testing.T) {
	// Arrange.
	var traffic atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		traffic.Add(1)
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL)
	// Act.
	err := transport.Start(t.Context())
	// Assert.
	require.Error(t, err)
	require.Zero(t, traffic.Load())
	require.NoError(t, transport.Close())
}

func TestMigratedSSEByteLimitRemainsCumulative(t *testing.T) {
	// Arrange: individually small comment frames exceed the total stream limit.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for range 50 {
			_, _ = fmt.Fprintf(w, ": %s\n\n", strings.Repeat("x", 100))
		}
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL,
		WithStreamableHTTPAllowPrivateIPs(true), WithStreamableHTTPMaxStreamBytes(1024))
	require.NoError(t, transport.Start(t.Context()))
	t.Cleanup(func() { _ = transport.Close() })
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	// Act.
	require.NoError(t, pending.Deliver())
	_, awaitErr := pending.Await(t.Context())
	// Assert.
	require.ErrorIs(t, awaitErr, textprocessor.ErrReadLimitExceeded)
}

func TestMigratedSSECancelUnblocksRequestStream(t *testing.T) {
	// Arrange: producer has started a request-scoped stream with no terminal result.
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": active\n\n")
		w.(http.Flusher).Flush()
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(t.Context()))
	t.Cleanup(func() { _ = transport.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	t.Cleanup(cancel)
	pending, err := transport.PrepareRequest(ctx, MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request stream did not start")
	}
	// Act.
	cancel()
	_, awaitErr := pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, awaitErr, context.Canceled)
}

func TestMigratedSSEInterruptPrecedenceAtClientBoundary(t *testing.T) {
	for _, interrupt := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(interrupt.Error(), func(t *testing.T) {
			// Arrange: joined/wrapped errors must not be downgraded into a size violation.
			client := &Client{}
			composite := fmt.Errorf("stream: %w", errors.Join(textprocessor.ErrReadLimitExceeded, interrupt))
			// Act.
			err := client.mapCallReadLimitFor(context.Background(), composite, "MCP stream")
			// Assert.
			require.ErrorIs(t, err, interrupt)
		})
	}
}
