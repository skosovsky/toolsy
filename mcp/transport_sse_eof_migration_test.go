package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigratedSSEDiscardsUnfinishedTerminalAtEOF(t *testing.T) {
	for _, ending := range []string{"", "\n", "\r\n"} {
		t.Run(fmt.Sprintf("ending=%q", ending), func(t *testing.T) {
			// Arrange: valid correlated payload, but the final empty line is absent.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request Request
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("request decode: %v", err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(
					w,
					"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"resultType\":\"complete\"}}%s",
					request.ID,
					ending,
				)
			}))
			t.Cleanup(server.Close)
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(t.Context()))
			t.Cleanup(func() { _ = transport.Close() })
			pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, migrationDiscoveryParams())
			require.NoError(t, err)
			// Act.
			require.NoError(t, pending.Deliver())
			result, awaitErr := pending.Await(t.Context())
			// Assert: EOF is not an event boundary or successful completion.
			require.ErrorContains(t, awaitErr, "stream ended before terminal response")
			require.Empty(t, result)
		})
	}
}

func TestMigratedSSEUnfinishedNotificationsDoNotDispatch(t *testing.T) {
	cases := []struct {
		name, origin, method, params string
	}{
		{"ack", MethodSubscriptionsListen, MethodSubscriptionsAcknowledged,
			`{"_meta":{"io.modelcontextprotocol/subscriptionId":"listen"},"notifications":{}}`},
		{"progress", MethodToolsCall, MethodProgress, `{"progressToken":"p","progress":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: identical payloads with and without the terminating empty line.
			transport := NewStreamableHTTPTransport("https://example.test/mcp")
			transport.peer = newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
			t.Cleanup(func() { _ = transport.Close() })
			calls := 0
			transport.peer.setNotificationHandler(tc.method, func(json.RawMessage) { calls++ })
			frame := fmt.Sprintf("data: {\"jsonrpc\":\"2.0\",\"method\":%q,\"params\":%s}", tc.method, tc.params)
			provenance := &sseRequestProvenance{requestID: json.RawMessage(`"listen"`), method: tc.origin,
				progressToken: json.RawMessage(`"p"`), hasProgress: true}
			// Act.
			err := transport.consumeRequestSSE(t.Context(), strings.NewReader(frame+"\n"), provenance)
			// Assert: no handler or ACK state change for an unfinished event.
			require.ErrorContains(t, err, "before terminal response")
			require.Zero(t, calls)
			require.False(t, provenance.acknowledged)
			// Positive control: framing, not payload rejection, caused the zero dispatch.
			err = transport.consumeRequestSSE(t.Context(), strings.NewReader(frame+"\n\n"), provenance)
			require.ErrorContains(t, err, "before terminal response")
			require.Equal(t, 1, calls)
			require.Equal(t, tc.name == "ack", provenance.acknowledged)
		})
	}
}
