package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestStreamableHTTP_EmptySSEDataDoesNotCompleteRequest(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest(MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	t.Cleanup(func() { peer.close(ErrTransportClosed) })
	transport := &StreamableHTTPTransport{maxFrameBytes: httptool.DefaultMaxSSEStreamBytes, peer: peer}
	provenance := &sseRequestProvenance{requestID: pending.ID(), method: MethodServerDiscover}
	// Act.
	err = transport.consumeRequestSSE(t.Context(), strings.NewReader("id: primed\ndata:\n\n"), provenance)
	// Assert: cursor priming no longer creates a replay state or a completion.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "stream ended before terminal response")
	peer.pendingMu.Lock()
	require.Len(t, peer.pending, 1)
	peer.pendingMu.Unlock()
}

func TestStreamableHTTP_SSEAcceptsStandardLineEndingsAndLeadingBOM(t *testing.T) {
	// Arrange.
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", separator), func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest(MethodServerDiscover, migrationDiscoveryParams())
			require.NoError(t, err)
			transport := &StreamableHTTPTransport{
				maxFrameBytes: httptool.DefaultMaxSSEStreamBytes,
				peer:          peer,
			}
			stream := "\ufeffid: cursor" + separator +
				fmt.Sprintf(`data: {"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID()) +
				separator + separator

			// Act.
			consumeErr := transport.consumeRequestSSE(
				context.Background(),
				strings.NewReader(stream),
				&sseRequestProvenance{requestID: pending.ID(), method: MethodServerDiscover},
			)
			result, awaitErr := pending.Await(context.Background())

			// Assert.
			require.NoError(t, consumeErr)
			require.NoError(t, awaitErr)
			require.JSONEq(t, `{}`, string(result))
		})
	}
}

func TestStreamableHTTP_SSEDiscardsIncompleteEventAtEOF(t *testing.T) {
	// Arrange: original id:next frame lacks an empty event boundary.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest(MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	t.Cleanup(func() { peer.close(ErrTransportClosed) })
	transport := &StreamableHTTPTransport{maxFrameBytes: httptool.DefaultMaxSSEStreamBytes, peer: peer}
	provenance := &sseRequestProvenance{requestID: pending.ID(), method: MethodServerDiscover}
	stream := fmt.Sprintf("id: next\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n", pending.ID())
	// Act.
	err = transport.consumeRequestSSE(t.Context(), strings.NewReader(stream), provenance)
	// Assert: no response is dispatched and no replay mechanism is introduced.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "stream ended before terminal response")
	peer.pendingMu.Lock()
	require.Len(t, peer.pending, 1)
	peer.pendingMu.Unlock()
}

func TestStreamableHTTP_SSELineUsesConfiguredStreamLimit(t *testing.T) {
	// Arrange.
	const payloadSize = defaultTransportMaxFrameBytes + 1024
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest(MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	transport := &StreamableHTTPTransport{
		maxFrameBytes: 2 * payloadSize,
		peer:          peer,
	}
	result := strings.Repeat("x", payloadSize)
	stream := fmt.Sprintf(
		"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%q}\n\n",
		pending.ID(),
		result,
	)

	// Act.
	consumeErr := transport.consumeRequestSSE(
		context.Background(),
		strings.NewReader(stream),
		&sseRequestProvenance{requestID: pending.ID(), method: MethodServerDiscover},
	)
	raw, awaitErr := pending.Await(context.Background())

	// Assert.
	require.NoError(t, consumeErr)
	require.NoError(t, awaitErr)
	require.Len(t, raw, payloadSize+2)
}
