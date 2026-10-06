package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestSSERetryValuesCannotEnableReplay(t *testing.T) {
	// Arrange: retain all nine original retry strings, without a retry scheduler.
	for _, value := range []string{"0", "1", "120000", "600000", "+1", "1.5", "-1", " 1", "18446744073709551615"} {
		t.Run(value, func(t *testing.T) {
			var posts, others atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					others.Add(1)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				posts.Add(1)
				envelope := decodeRPCEnvelope(t, r)
				w.Header().Set("Content-Type", eventStreamMediaType)
				_, _ = fmt.Fprintf(
					w,
					"retry: %s\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n",
					value,
					envelope.ID,
				)
			}))
			t.Cleanup(server.Close)
			transport := newSSEMigrationTransport(t, server.URL)
			// Act.
			result, err := sseMigrationRequest(t.Context(), t, transport)
			// Assert: fields are inert, no follow-up request.
			require.NoError(t, err)
			require.JSONEq(t, "{}", string(result))
			require.NoError(t, transport.Close())
			require.EqualValues(t, 1, posts.Load())
			require.Zero(t, others.Load())
		})
	}
}

func TestStreamableHTTP_ForbiddenIncomingRequestCannotSelfDeadlock(t *testing.T) {
	// Arrange: original server-request/roots fixture must not solicit an error reply.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) != 1 || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\"server-request\",\"method\":\"roots/list\"}\n\n")
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	_, err := sseMigrationRequest(t.Context(), t, transport)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "server-initiated request is forbidden")
	// Act.
	done := make(chan struct{})
	go func() { _ = transport.Close(); close(done) }()
	// Assert.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("incoming request deadlocked Close")
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestStreamableHTTP_CancellationClosesOriginalWireRequestWithoutNotification(t *testing.T) {
	// Arrange: keep original ordered/request identity and wire-before-cancel ordering.
	var mu sync.Mutex
	var methods []string
	started, stopped := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCEnvelope(t, r)
		mu.Lock()
		methods = append(methods, envelope.Method)
		mu.Unlock()
		if envelope.Method != "ordered/request" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, ": active\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	pending, err := transport.PrepareRequest(ctx, "ordered/request", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	awaitAdversarialSignal(t, started)
	// Act.
	cancel()
	_, err = pending.Await(ctx)
	// Assert: HTTP cancellation closes only its body, not a notifications/cancelled POST.
	require.ErrorIs(t, err, context.Canceled)
	awaitAdversarialSignal(t, stopped)
	require.NoError(t, transport.Close())
	mu.Lock()
	require.Equal(t, []string{"ordered/request"}, methods)
	mu.Unlock()
}

func TestStreamableHTTP_DiscoveryDisconnectCannotResumeOrInitialize(t *testing.T) {
	// Arrange: retain original session and initialize-1/emptydata/retry25 priming frame.
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		posts.Add(1)
		w.Header().Set("Mcp-Session-Id", "resume-session")
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, "id: initialize-1\ndata:\nretry: 25\n\n")
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	// Act: current discovery can be sent, legacy initialize cannot.
	legacy, err := transport.PrepareRequest(t.Context(), "initialize", migrationDiscoveryParams())
	require.Nil(t, legacy)
	require.ErrorContains(t, err, "legacy method \"initialize\" is forbidden")
	require.Zero(t, posts.Load())
	result, err := sseMigrationRequest(t.Context(), t, transport)
	// Assert.
	require.Empty(t, result)
	require.ErrorContains(t, err, "stream ended before terminal response")
	require.NoError(t, transport.Close())
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_SessionAdvertisementsAreInert(t *testing.T) {
	// Arrange: keep session-a/session-b replacement and late-establishment values.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Empty(t, r.Header.Get("Mcp-Session-Id"))
		assert.Empty(t, r.Header.Get("Last-Event-ID"))
		envelope := decodeRPCEnvelope(t, r)
		if calls.Add(1) == 1 {
			w.Header().Set("Mcp-Session-Id", "session-a")
		} else {
			w.Header().Set("Mcp-Session-Id", "session-b")
		}
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, envelope.ID, json.RawMessage("{}"))
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	// Act/Assert: advertisements cannot create or replace client-owned state.
	for range 2 {
		result, err := sseMigrationRequest(t.Context(), t, transport)
		require.NoError(t, err)
		require.JSONEq(t, "{}", string(result))
	}
	fresh := newSSEMigrationTransport(t, server.URL)
	_, err := sseMigrationRequest(t.Context(), t, fresh)
	require.NoError(t, err)
	require.NoError(t, transport.Close())
	require.NoError(t, fresh.Close())
	require.EqualValues(t, 3, calls.Load())
}

func TestStreamableHTTP_DecoratorTargetGuardAndInertEventIDs(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(r *http.Request) error {
			r.URL.Host = "attacker.invalid"
			return nil
		}),
	)
	peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
	t.Cleanup(func() { peer.close(ErrTransportClosed) })
	pending, _, err := peer.beginRequest(MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	parser := &StreamableHTTPTransport{peer: peer, maxStreamBytes: 2048}
	provenance := &sseRequestProvenance{requestID: pending.ID(), method: MethodServerDiscover}
	// Act: original whitespace and empty event IDs cannot create replay state.
	request, decoratorErr := transport.buildRequest(t.Context(), nil, nil)
	preservedErr := parser.consumeRequestSSE(t.Context(), strings.NewReader("id:  cursor  \n\n"), provenance)
	boundaryErr := parser.consumeRequestSSE(t.Context(), strings.NewReader("id:\n\n"), provenance)
	// Assert.
	require.Nil(t, request)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, decoratorErr, &invalid)
	require.ErrorContains(t, decoratorErr, "decorator must not change request method, URL, or Host override")
	require.ErrorContains(t, preservedErr, "stream ended before terminal response")
	require.ErrorContains(t, boundaryErr, "stream ended before terminal response")
	peer.pendingMu.Lock()
	require.Len(t, peer.pending, 1)
	peer.pendingMu.Unlock()
}

func TestStreamableHTTP_ParallelDisconnectedPOSTsNeverReplay(t *testing.T) {
	// Arrange: both original stream-ID/retry75 frames start before disconnect.
	var posts, gets atomic.Int32
	started := make(chan string, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		posts.Add(1)
		envelope := decodeRPCEnvelope(t, r)
		started <- string(envelope.ID)
		<-release
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprintf(w, "id: stream-%s\nretry: 75\n\n", envelope.ID)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	a, err := adversarialDeliveredDiscovery(t.Context(), t, transport)
	require.NoError(t, err)
	b, err := adversarialDeliveredDiscovery(t.Context(), t, transport)
	require.NoError(t, err)
	seen := map[string]bool{}
	for range 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("both POSTs did not start")
		}
	}
	// Act.
	close(release)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, errA := a.Await(ctx)
	_, errB := b.Await(ctx)
	// Assert: distinct pending identities both settle without retry.
	require.True(t, seen[string(a.ID())])
	require.True(t, seen[string(b.ID())])
	for _, awaitErr := range []error{errA, errB} {
		var invalid *InvalidPayloadError
		require.ErrorAs(t, awaitErr, &invalid)
		require.ErrorContains(t, awaitErr, "stream ended before terminal response")
	}
	require.NoError(t, transport.Close())
	require.EqualValues(t, 2, posts.Load())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_DisconnectFailsBeforeResume405(t *testing.T) {
	// Arrange: retain original stranded/retry1 and fallback405.
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		posts.Add(1)
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, "id: stranded\nretry: 1\n\n")
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	// Act.
	_, err := sseMigrationRequest(t.Context(), t, transport)
	// Assert: EOF, not a fabricated405 or timeout.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "stream ended before terminal response")
	require.NoError(t, transport.Close())
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_CancelledPendingStopsOriginPOSTWithoutResumeGET(t *testing.T) {
	// Arrange: original cancellable/retry1 stream now belongs to originating POST.
	started, stopped := make(chan struct{}), make(chan struct{})
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, "id: cancellable\nretry: 1\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	pending, err := adversarialDeliveredDiscovery(t.Context(), t, transport)
	require.NoError(t, err)
	awaitAdversarialSignal(t, started)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act: cancelling only Await still retires actual active stream.
	_, err = pending.Await(ctx)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	awaitAdversarialSignal(t, stopped)
	require.NoError(t, transport.Close())
	transport.mu.RLock()
	require.Empty(t, transport.activePosts)
	transport.mu.RUnlock()
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_ConnectNeverActivatesResumeCursor(t *testing.T) {
	// Arrange: original initialize-priming/emptydata/retry1 frame cannot bootstrap Connect.
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		posts.Add(1)
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, "id: initialize-priming\ndata:\nretry: 1\n\n")
	}))
	t.Cleanup(server.Close)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	// Act.
	client, err := Connect(ctx, transport)
	// Assert: no initialized notification, GET activation or cursor recovery.
	require.Nil(t, client)
	require.ErrorContains(t, err, "stream ended before terminal response")
	require.NoError(t, transport.Close())
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_ForgedSessionHeadersCannotBeDecorated(t *testing.T) {
	// Arrange: retain all four original forged values; every session header is forbidden.
	for _, session := range []string{"contains space", "control\x1f", "delete\x7f", string([]byte{0xff})} {
		t.Run(fmt.Sprintf("%q", session), func(t *testing.T) {
			transport := NewStreamableHTTPTransport(
				"https://example.invalid/mcp",
				WithStreamableHTTPRequestDecorator(func(r *http.Request) error {
					r.Header.Set("Mcp-Session-Id", session)
					return nil
				}),
			)
			// Act.
			request, err := transport.buildRequest(t.Context(), nil, nil)
			// Assert: reject before sending, without an obsolete session validator/state.
			require.Nil(t, request)
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.ErrorContains(t, err, "decorator must not set reserved MCP header")
		})
	}
}

func TestStreamableHTTP_OversizedPOSTSSEFailsClosedWithoutRetry(t *testing.T) {
	// Arrange: retain original256-byte data and32-byte stream cap.
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		posts.Add(1)
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", 256))
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL, WithStreamableHTTPMaxStreamBytes(32))
	// Act.
	_, err := sseMigrationRequest(t.Context(), t, transport)
	// Assert: original limit, not invalid JSON, closes peer with no retry.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Eventually(t, func() bool { return transport.peer.closed.Load() }, time.Second, time.Millisecond)
	after, afterErr := transport.PrepareRequest(t.Context(), MethodServerDiscover, migrationDiscoveryParams())
	require.Nil(t, after)
	require.ErrorIs(t, afterErr, ErrTransportClosed)
	require.NoError(t, transport.Close())
	require.EqualValues(t, 1, posts.Load())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_TerminalFailureCancelsConcurrentPOSTLifecycle(t *testing.T) {
	// Arrange: former activeGET becomes second POST; fatal400 must cancel both.
	started, stopped := make(chan struct{}, 2), make(chan struct{}, 2)
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			gets.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		envelope := decodeRPCEnvelope(t, r)
		if envelope.Method == "terminal/request" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", eventStreamMediaType)
		_, _ = fmt.Fprint(w, ": active\n\n")
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
		stopped <- struct{}{}
	}))
	t.Cleanup(server.Close)
	transport := newSSEMigrationTransport(t, server.URL)
	active := make([]PreparedRequest, 0, 2)
	for _, method := range []string{"active/request", "active/stream"} {
		pending, err := transport.PrepareRequest(t.Context(), method, migrationDiscoveryParams())
		require.NoError(t, err)
		require.NoError(t, pending.Deliver())
		active = append(active, pending)
		awaitAdversarialSignal(t, started)
	}
	terminal, err := transport.PrepareRequest(t.Context(), "terminal/request", migrationDiscoveryParams())
	require.NoError(t, err)
	// Act.
	require.NoError(t, terminal.Deliver())
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, terminalErr := terminal.Await(ctx)
	// Assert: exact400 propagates to all pending and closes active bodies.
	var terminalHTTP *HTTPError
	require.ErrorAs(t, terminalErr, &terminalHTTP)
	require.Equal(t, http.StatusBadRequest, terminalHTTP.StatusCode)
	for _, pending := range active {
		_, activeErr := pending.Await(ctx)
		var activeHTTP *HTTPError
		require.ErrorAs(t, activeErr, &activeHTTP)
		require.Equal(t, terminalHTTP.StatusCode, activeHTTP.StatusCode)
		awaitAdversarialSignal(t, stopped)
	}
	after, afterErr := transport.PrepareRequest(t.Context(), "after/terminal", migrationDiscoveryParams())
	require.Nil(t, after)
	require.ErrorIs(t, afterErr, ErrTransportClosed)
	require.NoError(t, transport.Close())
	require.Zero(t, gets.Load())
}

func TestStreamableHTTP_NotificationsForbiddenRegardlessOfResponseShape(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
	}{
		{"202 with body", http.StatusAccepted, "{}"},
		{"notification with 200", http.StatusOK, ""},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange: retain original202+body and200+emptybody fixtures.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(fixture.status)
				_, _ = fmt.Fprint(w, fixture.body)
			}))
			t.Cleanup(server.Close)
			transport := newSSEMigrationTransport(t, server.URL)
			// Act.
			err := transport.Notify(t.Context(), "notifications/test", struct{}{})
			// Assert: valid notification cannot reach either fixture through a legacy path.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.ErrorContains(t, err, "client notifications are not defined")
			require.Zero(t, calls.Load())
			_, responseErr := sseMigrationRequest(t.Context(), t, transport)
			if fixture.status == http.StatusAccepted {
				require.ErrorContains(t, responseErr, "202 Accepted is invalid")
			} else {
				require.ErrorContains(t, responseErr, "Content-Type")
			}
			require.NoError(t, transport.Close())
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func awaitAdversarialSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("adversarial lifecycle signal timed out")
	}
}
