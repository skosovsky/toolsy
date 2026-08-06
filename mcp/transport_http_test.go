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

func TestStreamableHTTP_JSONSessionVersionAndDeleteContract(t *testing.T) {
	// Arrange.
	var mu sync.Mutex
	var initializedHeaders http.Header
	deleteCalled := false
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.Method {
			case http.MethodGet:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			case http.MethodDelete:
				mu.Lock()
				deleteCalled = true
				mu.Unlock()
				assert.Equal(t, "session-1", request.Header.Get(mcpSessionIDHeader))
				writer.WriteHeader(http.StatusNoContent)
			case http.MethodPost:
				envelope := decodeRPCEnvelope(t, request)
				if envelope.Method == MethodInitialize {
					assert.Empty(t, request.Header.Get(mcpProtocolVersionHeader))
					writer.Header().Set(mcpSessionIDHeader, "session-1")
					writer.Header().Set("Content-Type", "application/json")
					writeResponse(t, writer, envelope.ID, initializeResult(ServerCapabilities{}))
					return
				}
				mu.Lock()
				initializedHeaders = request.Header.Clone()
				mu.Unlock()
				writer.WriteHeader(http.StatusAccepted)
			}
		}),
	)
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(
		context.Background(),
		MethodInitialize,
		validInitializeParams(),
	)
	require.NoError(t, err)
	_, err = pending.Await(context.Background())
	require.NoError(t, err)
	transport.SetProtocolVersion(ProtocolVersion)
	transport.Activate()

	// Act.
	err = transport.Notify(context.Background(), MethodInitialized, struct{}{})
	closeErr := transport.Close()

	// Assert.
	require.NoError(t, err)
	require.NoError(t, closeErr)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, ProtocolVersion, initializedHeaders.Get(mcpProtocolVersionHeader))
	require.Equal(t, "session-1", initializedHeaders.Get(mcpSessionIDHeader))
	require.Contains(t, initializedHeaders.Get("Accept"), "application/json")
	require.Contains(t, initializedHeaders.Get("Accept"), "text/event-stream")
	require.True(t, deleteCalled)
}

func TestStreamableHTTP_POSTAcceptsSSEResponse(t *testing.T) {
	// Arrange.
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			fmt.Fprintf(
				writer,
				"id: event-1\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n\n",
			)
		}),
	)
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)

	// Act.
	result, err := pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(result))
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_GETResumesAndHandlesServerRequest(t *testing.T) {
	// Arrange.
	var getCount atomic.Int32
	responseReceived := make(chan Response, 1)
	resumeHeader := make(chan string, 1)
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.Method {
			case http.MethodGet:
				count := getCount.Add(1)
				if count == 1 {
					writer.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(
						writer,
						"id: event-a\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"server-1\",\"method\":\"roots/list\"}\n\n",
					)
					return
				}
				resumeHeader <- request.Header.Get("Last-Event-ID")
				writer.WriteHeader(http.StatusMethodNotAllowed)
			case http.MethodPost:
				envelope := decodeRPCEnvelope(t, request)
				if envelope.Method == MethodInitialize {
					writer.Header().Set("Content-Type", "application/json")
					writeResponse(t, writer, envelope.ID, initializeResult(ServerCapabilities{}))
					return
				}
				if len(envelope.ID) > 0 && envelope.Method == "" {
					responseReceived <- Response{ID: envelope.ID, Result: envelope.Result, Error: envelope.Error}
				}
				writer.WriteHeader(http.StatusAccepted)
			default:
				writer.WriteHeader(http.StatusNoContent)
			}
		}),
	)
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	transport.OnRequest(func(_ context.Context, request Request) (json.RawMessage, *JSONRPCError) {
		require.Equal(t, MethodRootsList, request.Method)
		return json.RawMessage(`{"roots":[]}`), nil
	})
	pending, err := transport.Request(
		context.Background(),
		MethodInitialize,
		validInitializeParams(),
	)
	require.NoError(t, err)
	_, err = pending.Await(context.Background())
	require.NoError(t, err)

	// Act.
	transport.SetProtocolVersion(ProtocolVersion)
	transport.Activate()

	// Assert.
	select {
	case response := <-responseReceived:
		require.JSONEq(t, `"server-1"`, string(response.ID))
		require.JSONEq(t, `{"roots":[]}`, string(response.Result))
	case <-time.After(2 * time.Second):
		t.Fatal("server request response was not posted")
	}
	select {
	case header := <-resumeHeader:
		require.Equal(t, "event-a", header)
	case <-time.After(2 * time.Second):
		t.Fatal("GET stream was not resumed")
	}
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_MalformedIncomingRequestReceivesInvalidRequest(t *testing.T) {
	// Arrange.
	protocolError := make(chan Response, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			writer.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(writer, "data: {\"jsonrpc\":\"1.0\",\"id\":\"server-invalid\",\"method\":\"roots/list\"}\n\n")
		case http.MethodPost:
			envelope := decodeRPCEnvelope(t, request)
			if envelope.Method == MethodInitialize {
				writer.Header().Set("Content-Type", "application/json")
				writeResponse(t, writer, envelope.ID, initializeResult(ServerCapabilities{}))
				return
			}
			if envelope.Error != nil {
				protocolError <- Response{ID: envelope.ID, Error: envelope.Error}
			}
			writer.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), MethodInitialize, validInitializeParams())
	require.NoError(t, err)
	_, err = pending.Await(context.Background())
	require.NoError(t, err)
	transport.SetProtocolVersion(ProtocolVersion)

	// Act.
	transport.Activate()

	// Assert.
	select {
	case response := <-protocolError:
		require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
		require.JSONEq(t, `"server-invalid"`, string(response.ID))
	case <-time.After(2 * time.Second):
		t.Fatal("invalid request response was not posted")
	}
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_SessionExpiryAndLegacyEndpointFailClosed(t *testing.T) {
	t.Run("session expiry", func(t *testing.T) {
		// Arrange.
		var calls atomic.Int32
		server := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if calls.Add(1) == 1 {
					envelope := decodeRPCEnvelope(t, request)
					writer.Header().Set(mcpSessionIDHeader, "session")
					writer.Header().Set("Content-Type", "application/json")
					writeResponse(t, writer, envelope.ID, initializeResult(ServerCapabilities{}))
					return
				}
				writer.WriteHeader(http.StatusNotFound)
			}),
		)
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		initialize, err := transport.Request(context.Background(), MethodInitialize, nil)
		require.NoError(t, err)
		_, err = initialize.Await(context.Background())
		require.NoError(t, err)

		// Act.
		pending, err := transport.Request(context.Background(), "next", nil)
		require.NoError(t, err)
		_, err = pending.Await(context.Background())

		// Assert.
		require.ErrorIs(t, err, ErrSessionExpired)
		require.NoError(t, transport.Close())
	})

	t.Run("legacy endpoint", func(t *testing.T) {
		// Arrange.
		server := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(writer, "event: endpoint\ndata: /message\n\n")
			}),
		)
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		pending, err := transport.Request(context.Background(), "test", nil)
		require.NoError(t, err)

		// Act.
		_, err = pending.Await(context.Background())

		// Assert.
		var payloadErr *InvalidPayloadError
		require.ErrorAs(t, err, &payloadErr)
		require.NoError(t, transport.Close())
	})
}

func decodeRPCEnvelope(t *testing.T, request *http.Request) rpcEnvelope {
	t.Helper()
	var envelope rpcEnvelope
	require.NoError(t, json.NewDecoder(request.Body).Decode(&envelope))
	return envelope
}

func writeResponse(t *testing.T, writer http.ResponseWriter, id, result json.RawMessage) {
	t.Helper()
	require.NoError(
		t,
		json.NewEncoder(writer).Encode(Response{JSONRPC: JSONRPCVersion, ID: id, Result: result}),
	)
}
