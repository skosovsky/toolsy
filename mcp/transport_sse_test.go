package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestSSEMigration_ConfiguredPostURL_BlocksPrivateIP(t *testing.T) {
	t.Parallel()
	// Arrange. URI resolution belongs to the caller, never endpoint discovery.
	base, err := url.Parse("http://example.com/sse")
	require.NoError(t, err)
	target, err := url.Parse("http://127.0.0.1:8080/rpc")
	require.NoError(t, err)
	resolved := base.ResolveReference(target)
	// Act.
	err = httptool.ValidateRemoteURL(context.Background(), resolved.String(), false)
	transport := NewStreamableHTTPTransport(resolved.String())
	startErr := transport.Start(context.Background())
	// Assert.
	require.Error(t, err)
	require.Error(t, startErr)
	require.NoError(t, transport.Close())
}
func TestSSEMigration_ConfiguredPostURL_AllowPrivateIPs(t *testing.T) {
	t.Parallel()
	// Arrange.
	base, err := url.Parse("http://example.com/sse")
	require.NoError(t, err)
	target, err := url.Parse("http://127.0.0.1:8080/rpc")
	require.NoError(t, err)
	resolved := base.ResolveReference(target)
	// Act.
	err = httptool.ValidateRemoteURL(context.Background(), resolved.String(), true)
	transport := NewStreamableHTTPTransport(resolved.String(), WithStreamableHTTPAllowPrivateIPs(true))
	startErr := transport.Start(context.Background())
	// Assert. Start performs no dial or GET.
	require.NoError(t, err)
	require.NoError(t, startErr)
	require.NoError(t, transport.Close())
}
func TestSSEMigration_ValidateInitialURL_BlocksPrivateIP(t *testing.T) {
	t.Parallel()
	// Arrange / Act / Assert.
	err := httptool.ValidateRemoteURL(context.Background(), "http://127.0.0.1/sse", false)
	require.Error(t, err)
	transport := NewStreamableHTTPTransport("http://127.0.0.1/sse")
	require.Error(t, transport.Start(context.Background()))
	require.NoError(t, transport.Close())
}
func TestSSEMigration_HTTPNotificationsForbiddenWith500Fixture(t *testing.T) {
	t.Parallel()
	// Arrange. Retain the original HTTP500 fixture and notifications/test method.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/messages", r.URL.Path)
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL+"/messages")
	// Act. The explicit POST proves the fixture; a notification must add no traffic.
	_, postErr := sseMigrationRequest(context.Background(), t, transport)
	notifyErr := transport.Notify(context.Background(), "notifications/test", struct{}{})
	// Assert.
	var httpErr *HTTPError
	require.ErrorAs(t, postErr, &httpErr)
	require.Equal(t, 500, httpErr.StatusCode)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, notifyErr, &invalid)
	require.ErrorContains(t, notifyErr, "client notifications are not defined")
	require.EqualValues(t, 1, calls.Load())
}
func TestSSEMigration_Call_Non2xxPOSTStatus(t *testing.T) {
	t.Parallel()
	// Arrange.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "messages") {
			http.Error(w, "fail", http.StatusBadRequest)
			return
		}
		http.Error(w, "unexpected", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL+"/messages")
	// Act.
	_, err := sseMigrationRequest(context.Background(), t, transport)
	// Assert.
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, 400, httpErr.StatusCode)
}
func TestSSEMigration_StartDoesNotGET500Endpoint(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	// Act / Assert. Start cannot depend on GET status.
	transport := newSSEMigrationTransport(t, server.URL)
	require.Zero(t, calls.Load())
	_, err := sseMigrationRequest(context.Background(), t, transport)
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, 500, httpErr.StatusCode)
	require.EqualValues(t, 1, calls.Load())
}
func TestSSEMigration_StartDoesNotGET204Endpoint(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	require.Zero(t, calls.Load())
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// Act.
	_, err := sseMigrationRequest(ctx, t, transport)
	// Assert. HTTP204 with no terminal body is not a successful RPC.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "Content-Type")
	require.EqualValues(t, 1, calls.Load())
}
func TestSSEMigration_Start_PreCancelledWithoutEndpointWait(t *testing.T) {
	t.Parallel()
	// Arrange. Keep the original keepalive/no-endpoint server, but never contact it.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	err := transport.Start(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
	require.NoError(t, transport.Close())
}
func TestSSEMigration_Call_CancelUnblocksPending(t *testing.T) {
	t.Parallel()
	// Arrange. Request-scoped keepalive stream replaces the obsolete GET stream.
	accepted, finished := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		w.(http.Flusher).Flush()
		close(accepted)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, MethodToolsList, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	select {
	case <-accepted:
	case <-ctx.Done():
		t.Fatal("POST was not accepted")
	}
	// Act.
	cancel()
	_, err = pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("caller cancellation did not close response stream")
	}
	require.EqualValues(t, 1, calls.Load())
}
func TestSSEMigration_ExceedsMaxStreamBytes(t *testing.T) {
	t.Parallel()
	// Arrange. Same 50x200-byte payload volume, now comments rather than invalid RPC data.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for range 50 {
			fmt.Fprintf(w, ": %s\n\n", strings.Repeat("x", 200))
		}
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL, WithStreamableHTTPMaxStreamBytes(2048))
	// Act.
	_, err := sseMigrationRequest(context.Background(), t, transport)
	// Assert.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestSSEMigration_OriginalEndpointAndMalformedDataFramesFailClosed(t *testing.T) {
	// Keep the original removed-transport control/data bytes as explicit negatives,
	// independently from the cumulative-volume comment fixture.
	frames := []string{
		"event: endpoint\ndata: /messages\n\n",
		"event: endpoint\ndata: http://example.com/messages\n\n",
		fmt.Sprintf("data: %s\n\n", strings.Repeat("x", 200)),
	}
	for index, frame := range frames {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, frame)
			}))
			t.Cleanup(server.Close)
			transport := newSSEMigrationTransport(t, server.URL)
			// Act.
			_, err := sseMigrationRequest(context.Background(), t, transport)
			// Assert. Parsing failure, not ignored-frame EOF or endpoint following.
			var invalid *InvalidPayloadError
			var syntaxErr *json.SyntaxError
			require.ErrorAs(t, err, &invalid)
			require.ErrorAs(t, err, &syntaxErr)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
func TestSSEMigration_ResultBoundary_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&Client{}).mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "MCP SSE response")
	require.ErrorIs(t, err, context.Canceled)
}
func TestSSEMigration_ResultBoundary_CancelOverStaleStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&Client{}).mapCallReadLimitFor(ctx, errors.New("sse stream closed"), "MCP SSE response")
	require.ErrorIs(t, err, context.Canceled)
}
func TestSSEMigration_ResultBoundary_LimitWithoutCancel(t *testing.T) {
	t.Parallel()
	err := (&Client{}).mapCallReadLimitFor(context.Background(), textprocessor.ErrReadLimitExceeded, "MCP SSE response")
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	typed, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, typed.Code)
	require.Contains(t, typed.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}
func TestSSEMigration_ResultBoundary_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf("stream: %w", errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded))
	err := (&Client{}).mapCallReadLimitFor(context.Background(), composite, "MCP SSE response")
	require.ErrorIs(t, err, context.Canceled)
	require.Same(t, composite, err)
}
func TestSSEMigration_TerminalInterruptOverReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange. Terminal failure must reach an already-prepared waiter.
	transport := newSSEMigrationTransport(t, "http://127.0.0.1:8080/rpc")
	pending, err := transport.PrepareRequest(context.Background(), MethodToolsList, migrationDiscoveryParams())
	require.NoError(t, err)
	cause := fmt.Errorf("stream: %w", errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded))
	// Act.
	transport.terminate(cause)
	_, err = pending.Await(context.Background())
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
func TestSSEMigration_StreamLimitPrecedesForbiddenEndpoint(t *testing.T) {
	t.Parallel()
	// Arrange. No endpoint frame after exhausted bytes may be followed.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "text/event-stream")
		for range 50 {
			fmt.Fprintf(w, ": %s\n\n", strings.Repeat("x", 200))
		}
		fmt.Fprint(w, "event: endpoint\ndata: http://example.com/messages\n\n")
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL, WithStreamableHTTPMaxStreamBytes(2048))
	// Act.
	_, err := sseMigrationRequest(context.Background(), t, transport)
	// Assert.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.EqualValues(t, 1, calls.Load())
}
func TestSSEMigration_PreCancelledPrepareOverTerminalLimit(t *testing.T) {
	t.Parallel()
	// Arrange.
	transport := newSSEMigrationTransport(t, "http://example.com/messages")
	transport.terminate(textprocessor.ErrReadLimitExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	pending, err := transport.PrepareRequest(ctx, MethodToolsList, migrationDiscoveryParams())
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, pending)
	require.Zero(t, transport.peer.requestID.Load())
}
func TestSSEMigration_PreCancelledCallOverTerminalLimit(t *testing.T) {
	t.Parallel()
	// Arrange.
	transport := newSSEMigrationTransport(t, "http://example.com/messages")
	transport.terminate(textprocessor.ErrReadLimitExceeded)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act. The public client boundary retains caller cancellation, never retries.
	client := &Client{transport: transport}
	err := client.mapCallReadLimitFor(ctx, transport.failureCause(ErrTransportClosed), "MCP SSE response")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
func TestSSEMigration_ResultBoundary_DeadlineOverReadLimit(t *testing.T) {
	t.Parallel()
	// Arrange / Act.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancel()
	err := (&Client{}).mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "MCP SSE response")
	// Assert.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func newSSEMigrationTransport(t *testing.T, endpoint string, opts ...StreamableHTTPOption) *StreamableHTTPTransport {
	t.Helper()
	options := append([]StreamableHTTPOption{WithStreamableHTTPAllowPrivateIPs(true)}, opts...)
	transport := NewStreamableHTTPTransport(endpoint, options...)
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	return transport
}

func sseMigrationRequest(
	parent context.Context,
	t *testing.T,
	transport *StreamableHTTPTransport,
) (json.RawMessage, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, MethodToolsList, migrationDiscoveryParams())
	if err != nil {
		return nil, err
	}
	if err = pending.Deliver(); err != nil {
		return nil, err
	}
	return pending.Await(ctx)
}
