package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// adversarialDeliveredDiscovery explicitly exercises the current prepare/deliver
// lifecycle. It is test-only and does not accept or rewrite legacy wire methods.
func adversarialDeliveredDiscovery(ctx context.Context, t *testing.T, transport Transport) (PreparedRequest, error) {
	t.Helper()
	pending, err := transport.PrepareRequest(ctx, MethodServerDiscover, migrationDiscoveryParams())
	if err != nil {
		return nil, err
	}
	if err := pending.Deliver(); err != nil {
		return nil, err
	}
	return pending, nil
}

func TestStreamableHTTP_CloseCompletesHeldRequestWithTransportClosed(t *testing.T) {
	// Arrange.
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", eventStreamMediaType)
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		close(received)
		<-request.Context().Done()
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
	require.NoError(t, err)
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("server did not receive held request")
	}

	// Act.
	require.NoError(t, transport.Close())
	_, err = pending.Await(context.Background())

	// Assert.
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestStreamableHTTP_PreCancelledRequestNeverAllocatesOrWritesID(t *testing.T) {
	// Arrange.
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act.
	pending, err := transport.PrepareRequest(ctx, MethodServerDiscover, migrationDiscoveryParams())

	// Assert.
	require.Nil(t, pending)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(0), requests.Load())
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_DecoratorCannotOverrideRequestHost(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			request.Host = "internal-vhost"
			return nil
		}),
	)

	// Act.
	request, err := transport.buildRequest(context.Background(), nil, nil)

	// Assert.
	require.Nil(t, request)
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
}

func TestStreamableHTTP_DecoratorCannotSetBodyContract(t *testing.T) {
	mutations := map[string]func(*http.Request){
		"body": func(request *http.Request) { request.Body = http.NoBody },
		"get body": func(request *http.Request) {
			request.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		},
		"content length":    func(request *http.Request) { request.ContentLength = 1 },
		"transfer encoding": func(request *http.Request) { request.TransferEncoding = []string{"chunked"} },
		"trailer":           func(request *http.Request) { request.Trailer = http.Header{"X-Test": {"value"}} },
		"length header":     func(request *http.Request) { request.Header.Set("Content-Length", "1") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			transport := NewStreamableHTTPTransport(
				"https://example.invalid/mcp",
				WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
					mutate(request)
					return nil
				}),
			)

			// Act.
			request, err := transport.buildRequest(
				context.Background(),
				[]byte(`{"jsonrpc":"2.0"}`),
				nil,
			)

			// Assert.
			require.Nil(t, request)
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
		})
	}
}

func TestStreamableHTTP_RequestIsDetachedFromDecoratorPointer(t *testing.T) {
	// Arrange.
	var decorated *http.Request
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			request.Header.Set("Authorization", "Bearer original")
			decorated = request
			return nil
		}),
	)
	request, err := transport.buildRequest(
		context.Background(),
		[]byte(`{"jsonrpc":"2.0"}`),
		nil,
	)
	require.NoError(t, err)

	// Act.
	decorated.Header.Set("Authorization", "Bearer forged")
	decorated.URL.Host = "attacker.invalid"
	decorated.Body = http.NoBody
	body, readErr := io.ReadAll(request.Body)

	// Assert.
	require.NoError(t, readErr)
	require.Equal(t, "Bearer original", request.Header.Get("Authorization"))
	require.Equal(t, "example.invalid", request.URL.Host)
	require.JSONEq(t, `{"jsonrpc":"2.0"}`, string(body))
}

func TestStreamableHTTP_RejectsPrivateEndpointAndOversizedBody(t *testing.T) {
	// Arrange.
	private := NewStreamableHTTPTransport("http://127.0.0.1:1/mcp")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":{"padding":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}`)
	}))
	defer server.Close()
	bounded := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
		WithStreamableHTTPMaxStreamBytes(32),
	)
	require.NoError(t, bounded.Start(context.Background()))
	pending, err := adversarialDeliveredDiscovery(context.Background(), t, bounded)
	require.NoError(t, err)

	// Act.
	privateErr := private.Start(context.Background())
	_, bodyErr := pending.Await(context.Background())

	// Assert.
	require.Error(t, privateErr)
	require.ErrorIs(t, bodyErr, textprocessor.ErrReadLimitExceeded)
	require.NoError(t, bounded.Close())
}

func TestStreamableHTTP_RequestRejects202AndCloseCancelsActivePOST(t *testing.T) {
	t.Run("request rejects 202", func(t *testing.T) {
		// Arrange.
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
		require.NoError(t, err)

		// Act.
		_, err = pending.Await(context.Background())

		// Assert.
		var payloadErr *InvalidPayloadError
		require.ErrorAs(t, err, &payloadErr)
		require.ErrorContains(t, err, "202 Accepted is invalid")
		require.NoError(t, transport.Close())
	})

	t.Run("close cancels post", func(t *testing.T) {
		// Arrange.
		started := make(chan struct{})
		releaseServer := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			select {
			case <-request.Context().Done():
			case <-releaseServer:
			}
		}))
		defer server.Close()
		defer close(releaseServer)
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
		require.NoError(t, err)
		<-started

		// Act.
		require.NoError(t, transport.Close())
		_, pendingErr := pending.Await(context.Background())

		// Assert.
		require.ErrorIs(t, pendingErr, ErrTransportClosed)
		transport.mu.RLock()
		activePosts := len(transport.activePosts)
		transport.mu.RUnlock()
		require.Zero(t, activePosts, "Close returned before the active POST stopped")
	})
}

func TestStreamableHTTP_JSONPOSTRequiresCorrelatedTerminalResponse(t *testing.T) {
	fixtures := []struct {
		name     string
		response string
	}{
		{
			name:     "notification",
			response: `{"jsonrpc":"2.0","method":"notifications/message","params":{}}`,
		},
		{name: "mismatched id", response: `{"jsonrpc":"2.0","id":999,"result":{}}`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, fixture.response)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
			require.NoError(t, err)

			// Act.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = pending.Await(ctx)

			// Assert.
			var payloadErr *InvalidPayloadError
			require.ErrorAs(t, err, &payloadErr)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_RejectsInvalidUTF8JSONAndSSE(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{
			name:        "json",
			contentType: "application/json",
			body:        []byte{'{', '"', 'j', 's', 'o', 'n', 'r', 'p', 'c', '"', ':', '"', 0xff, '"', '}'},
		},
		{name: "sse", contentType: "text/event-stream", body: []byte{'d', 'a', 't', 'a', ':', ' ', 0xff, '\n', '\n'}},
		{name: "correlated invalid UTF8", contentType: "application/json"},
		{name: "correlated ASCII control", contentType: "application/json"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", fixture.contentType)
				body, bodyErr := utf8MigrationResponse(request, fixture.name, fixture.body)
				if bodyErr != nil {
					t.Errorf("decode request: %v", bodyErr)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = writer.Write(body)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
			require.NoError(t, err)

			// Act.
			_, err = pending.Await(context.Background())

			// Assert.
			if fixture.name == "correlated ASCII control" {
				require.NoError(t, err)
				require.NoError(t, transport.Close())
				return
			}
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			if fixture.name == "json" {
				// Original fixture also lacks an ID, rejected before payload dispatch.
				require.Equal(t, "Streamable HTTP response id", invalid.Subject)
			} else {
				require.ErrorContains(t, err, "not valid UTF-8")
			}
			require.NoError(t, transport.Close())
		})
	}
}

func utf8MigrationResponse(request *http.Request, name string, original []byte) ([]byte, error) {
	if original != nil {
		return original, nil
	}
	var envelope Request
	if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	body := fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":{"text":"`, envelope.ID)
	if name == "correlated invalid UTF8" {
		body = append(body, 0xff)
	} else {
		body = append(body, 'x')
	}
	return append(body, []byte(`"}}`)...), nil
}

func TestStreamableHTTP_TerminalSSEEventCancelsHeldOpenPOST(t *testing.T) {
	// Arrange.
	postCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var message struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", eventStreamMediaType)
		writer.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(
			writer,
			"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n",
			message.ID,
		)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
		close(postCancelled)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
	require.NoError(t, err)

	// Act.
	result, err := pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
	select {
	case <-postCancelled:
	case <-time.After(time.Second):
		t.Fatal("terminal SSE event did not cancel held-open POST")
	}
	require.Eventually(t, func() bool {
		transport.mu.RLock()
		defer transport.mu.RUnlock()
		return len(transport.activePosts) == 0
	}, time.Second, time.Millisecond)
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_RequiresExactMediaType(t *testing.T) {
	for _, contentType := range []string{"application/json-evil", "text/event-stream-bogus"} {
		t.Run(contentType, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", contentType)
				_, _ = fmt.Fprint(writer, `{}`)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
			require.NoError(t, err)

			// Act.
			_, err = pending.Await(context.Background())

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.ErrorContains(t, err, fmt.Sprintf("unsupported content type %q", contentType))
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_RejectsMethodChangingRedirect(t *testing.T) {
	// Arrange.
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls.Add(1)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer origin.Close()
	transport := NewStreamableHTTPTransport(origin.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
	require.NoError(t, err)

	// Act.
	_, err = pending.Await(context.Background())

	// Assert.
	var refused *httptool.RedirectError
	require.ErrorAs(t, err, &refused)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeRemoteExecution, te.Code)
	require.False(t, te.Retryable)
	require.False(t, toolsy.ClientCorrectable(te.Code))
	require.Zero(t, targetCalls.Load())
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_SafeClientRejectsRedirectToLoopback(t *testing.T) {
	// Arrange.
	client := defaultStreamableHTTPClient(false)
	redirect, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/private", nil)
	require.NoError(t, err)
	original, err := http.NewRequest(http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	// Act.
	err = client.CheckRedirect(redirect, []*http.Request{original})

	// Assert.
	require.Error(t, err)
}

func TestTransport_CloseBeforeStartIsTerminal(t *testing.T) {
	// Arrange.
	httpTransport := NewStreamableHTTPTransport("https://example.invalid/mcp")
	stdioTransport := NewStdioTransport("ignored", nil)

	// Act.
	require.NoError(t, httpTransport.Close())
	require.NoError(t, stdioTransport.Close())

	// Assert.
	require.ErrorIs(t, httpTransport.Start(context.Background()), ErrTransportClosed)
	require.ErrorIs(t, stdioTransport.Start(context.Background()), ErrTransportClosed)
	prepared, err := httpTransport.PrepareRequest(
		context.Background(),
		MethodServerDiscover,
		migrationDiscoveryParams(),
	)
	require.Nil(t, prepared)
	require.ErrorIs(t, err, ErrTransportClosed)
	require.ErrorContains(
		t,
		httpTransport.Notify(context.Background(), "notifications/test", struct{}{}),
		"client notifications are not defined",
	)
	require.ErrorIs(
		t,
		stdioTransport.Notify(context.Background(), "notifications/test", struct{}{}),
		ErrTransportClosed,
	)
}

func TestStdio_ProcessExitReturnsTypedCrash(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport("/bin/sh", []string{"-c", "read line; exit 7"})
	require.NoError(t, transport.Start(context.Background()))
	pending, err := adversarialDeliveredDiscovery(context.Background(), t, transport)
	require.NoError(t, err)

	// Act.
	_, err = pending.Await(context.Background())

	// Assert.
	var crashErr *TransportCrashError
	require.ErrorAs(t, err, &crashErr)
	require.NoError(t, transport.Close())
}

func TestStructuredContent_PreservesLargeInteger(t *testing.T) {
	// Arrange.
	raw := json.RawMessage(`{"value":9007199254740993}`)

	// Act.
	canonical, _, err := canonicalJSON(raw)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(canonical))
	require.Contains(t, string(canonical), "9007199254740993")
}

func TestResourceResult_PreservesSingleContentMIMEAndBinary(t *testing.T) {
	// Arrange.
	text := `{"ok":true}`
	blob := "AAE="

	// Act.
	jsonChunk, err := buildResourceResultChunk(
		ResourcesReadResult{
			Contents: []ResourceContents{
				{URI: "file:///a", MIMEType: toolsy.MimeTypeJSON, Text: &text},
			},
		},
		nil,
	)
	require.NoError(t, err)
	binaryChunk, err := buildResourceResultChunk(
		ResourcesReadResult{
			Contents: []ResourceContents{
				{URI: "file:///b", MIMEType: "application/octet-stream", Blob: &blob},
			},
		},
		nil,
	)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, toolsy.MimeTypeJSON, jsonChunk.MimeType)
	require.JSONEq(t, text, string(jsonChunk.Data))
	require.Equal(t, []byte{0, 1}, binaryChunk.Data)
	require.Equal(t, toolsy.DeliveryClassBinary, binaryChunk.Envelope.DeliveryClass)
}
