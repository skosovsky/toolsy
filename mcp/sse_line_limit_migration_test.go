package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestStreamableHTTP_SSELineBudgetOwnsScannerFailure(t *testing.T) {
	for _, size := range []int{31, 32, 256} {
		t.Run(strings.Repeat("x", size), func(t *testing.T) {
			// Arrange: valid comments isolate resource limits from JSON validation.
			peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
			t.Cleanup(func() { peer.close(ErrTransportClosed) })
			transport := &StreamableHTTPTransport{peer: peer, maxFrameBytes: 32}
			line := ":" + strings.Repeat("x", size-1)
			provenance := &sseRequestProvenance{method: MethodServerDiscover}
			// Act.
			err := transport.consumeRequestSSE(t.Context(), strings.NewReader(line), provenance)
			// Assert: frames are inclusive; oversized unterminated lines retain the typed limit cause.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			if size <= 32 {
				require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
				require.ErrorContains(t, err, "stream ended before terminal response")
			} else {
				require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
			}
		})
	}
	// Arrange: cancellation wins over the same budget-boundary input.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	transport := &StreamableHTTPTransport{maxFrameBytes: 32}
	// Act.
	err := transport.consumeRequestSSE(ctx, strings.NewReader(strings.Repeat("x", 256)), &sseRequestProvenance{})
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
}
