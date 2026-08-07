package mcp

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTask34RandomizedIntegralRPCIDsCanonicalizeWithoutStringCollision(t *testing.T) {
	// Arrange.
	state := uint64(0x6a09e667f3bcc909)

	for range 512 {
		state = state*6364136223846793005 + 1442695040888963407
		value := state % 1_000_000_000_000
		decimal := strconv.FormatUint(value, 10)
		integralForms := []string{decimal, decimal + ".0", decimal + "e0"}

		// Act.
		canonical, err := rpcIDKey(json.RawMessage(integralForms[0]))

		// Assert.
		require.NoError(t, err)
		for _, form := range integralForms[1:] {
			key, formErr := rpcIDKey(json.RawMessage(form))
			require.NoError(t, formErr, form)
			require.Equal(t, canonical, key, form)
		}
		stringKey, stringErr := rpcIDKey(json.RawMessage(strconv.Quote(decimal)))
		require.NoError(t, stringErr)
		require.NotEqual(t, canonical, stringKey)
		_, fractionalErr := rpcIDKey(json.RawMessage(decimal + ".5"))
		require.Error(t, fractionalErr)
	}
}

func TestTask34CancelledIDFragmentationRemainsBoundedAndFailsClosed(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
	peer.cancelledRanges = []rpcIDRange{{first: 1, last: 2*maxCancelledResponseRanges + 3}}

	// Act.
	for id := uint64(2); id < 2*maxCancelledResponseRanges; id += 2 {
		removed, overflow := peer.removeCancelledIDLocked(id)
		require.True(t, removed)
		require.False(t, overflow)
	}
	removed, overflow := peer.removeCancelledIDLocked(2 * maxCancelledResponseRanges)

	// Assert.
	require.False(t, removed)
	require.True(t, overflow)
	require.Len(t, peer.cancelledRanges, maxCancelledResponseRanges)
	peer.close(ErrTransportClosed)
}
