package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamableHTTP_JSONVersionIgnoresSessionAndNeverDeletes(t *testing.T) {
	// Arrange. The original session-1 advertisement is inert.
	var mu sync.Mutex
	var headers []http.Header
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers = append(headers, r.Header.Clone())
		methods = append(methods, r.Method)
		mu.Unlock()
		assert.Equal(t, http.MethodPost, r.Method)
		envelope := decodeRPCEnvelope(t, r)
		assert.Equal(t, MethodServerDiscover, envelope.Method)
		w.Header().Set("Mcp-Session-Id", "session-1")
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, envelope.ID, completeDiscovery(ServerCapabilities{}))
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))

	// Act. The second explicit request must not echo the advertised session.
	for range 2 {
		result, err := httpMigrationRequest(transport)
		require.NoError(t, err)
		require.JSONEq(t, string(completeDiscovery(ServerCapabilities{})), string(result))
	}
	closeErr := transport.Close()

	// Assert. Close cannot add DELETE, GET or legacy notifications.
	require.NoError(t, closeErr)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{http.MethodPost, http.MethodPost}, methods)
	for _, header := range headers {
		require.Equal(t, ProtocolVersion, header.Get("Mcp-Protocol-Version"))
		require.Empty(t, header.Get("Mcp-Session-Id"))
		require.Empty(t, header.Get("Last-Event-ID"))
		require.Contains(t, header.Get("Accept"), "application/json")
		require.Contains(t, header.Get("Accept"), "text/event-stream")
	}
}

func TestStreamableHTTP_POSTAcceptsSSEResponse(t *testing.T) {
	// Arrange. Retain the original event ID and result payload.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelope := decodeRPCEnvelope(t, r)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "id: event-1\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"ok\":true}}\n\n", envelope.ID)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))

	// Act.
	result, err := httpMigrationRequest(transport)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(result))
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_ServerRequestFailsWithoutGETResumeOrReply(t *testing.T) {
	// Arrange. Retain event-a, server-1 and roots/list; inject over supported POST.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Empty(t, r.Header.Get("Last-Event-ID"))
		assert.Equal(t, MethodServerDiscover, decodeRPCEnvelope(t, r).Method)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "id: event-a\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"server-1\",\"method\":\"roots/list\"}\n\n")
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))

	// Act.
	_, err := httpMigrationRequest(transport)
	closeErr := transport.Close()

	// Assert. No server reply, GET reconnect, Last-Event-ID or automatic retry.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "server-initiated request is forbidden")
	require.NoError(t, closeErr)
	require.EqualValues(t, 1, calls.Load())
}

func TestStreamableHTTP_MalformedIncomingRequestFailsWithoutReply(t *testing.T) {
	// Arrange. Retain the original invalid version and server-invalid ID.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, MethodServerDiscover, decodeRPCEnvelope(t, r).Method)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"jsonrpc\":\"1.0\",\"id\":\"server-invalid\",\"method\":\"roots/list\"}\n\n")
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))

	// Act.
	_, err := httpMigrationRequest(transport)
	closeErr := transport.Close()

	// Assert. The old unsolicited error-response path is forbidden.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "server-initiated request is forbidden")
	require.NoError(t, closeErr)
	require.EqualValues(t, 1, calls.Load())
}

func TestStreamableHTTP_HTTP404AndLegacyEndpointFailClosed(t *testing.T) {
	t.Run("session expiry", func(t *testing.T) {
		// Arrange. Retain session advertisement followed by HTTP 404.
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Empty(t, r.Header.Get("Mcp-Session-Id"))
			if calls.Add(1) == 1 {
				envelope := decodeRPCEnvelope(t, r)
				w.Header().Set("Mcp-Session-Id", "session")
				w.Header().Set("Content-Type", "application/json")
				writeResponse(t, w, envelope.ID, completeDiscovery(ServerCapabilities{}))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		_, err := httpMigrationRequest(transport)
		require.NoError(t, err)

		// Act.
		_, err = httpMigrationRequest(transport)
		closeErr := transport.Close()

		// Assert. Stateless failure is not a session recovery or a blind retry.
		var httpErr *HTTPError
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusNotFound, httpErr.StatusCode)
		require.Equal(t, "POST", httpErr.Operation)
		require.NoError(t, closeErr)
		require.EqualValues(t, 2, calls.Load())
	})
	t.Run("legacy endpoint", func(t *testing.T) {
		// Arrange.
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			assert.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: endpoint\ndata: /message\n\n")
		}))
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))

		// Act.
		_, err := httpMigrationRequest(transport)
		closeErr := transport.Close()

		// Assert. No endpoint switch, reconnect or secondary POST.
		var invalid *InvalidPayloadError
		require.ErrorAs(t, err, &invalid)
		var syntaxErr *json.SyntaxError
		require.ErrorAs(t, err, &syntaxErr)
		require.NoError(t, closeErr)
		require.EqualValues(t, 1, calls.Load())
	})
}

func httpMigrationRequest(transport *StreamableHTTPTransport) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pending, err := transport.PrepareRequest(ctx, MethodServerDiscover, migrationDiscoveryParams())
	if err != nil {
		return nil, err
	}
	if err = pending.Deliver(); err != nil {
		return nil, err
	}
	return pending.Await(ctx)
}

func decodeRPCEnvelope(t *testing.T, request *http.Request) rpcEnvelope {
	t.Helper()
	var envelope rpcEnvelope
	require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
	return envelope
}

func writeResponse(t *testing.T, w http.ResponseWriter, id, result json.RawMessage) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(Response{JSONRPC: JSONRPCVersion, ID: id, Result: result}))
}
