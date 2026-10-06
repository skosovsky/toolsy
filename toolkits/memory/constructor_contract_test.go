package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32MemoryLimits(t *testing.T) {
	for _, limit := range []func(int) Option{WithMaxFacts, WithMaxKeyBytes, WithMaxValueBytes, WithMaxStoreBytes, WithMaxOutputBytes} {
		// Arrange.
		negative, positive, zero := limit(-1), limit(1024), limit(0)
		// Act.
		pad, err := NewScratchpad(negative)
		// Assert.
		require.Error(t, err)
		require.Nil(t, pad)
		pad, err = NewScratchpad(zero)
		require.NoError(t, err)
		require.NotNil(t, pad)
		pad, err = NewScratchpad(negative, positive)
		require.NoError(t, err)
		require.NotNil(t, pad)
		pad, err = NewScratchpad(positive, negative)
		require.Error(t, err)
		require.Nil(t, pad)
	}
	// Nil must never panic, even after a valid option.
	require.NotPanics(t, func() {
		pad, err := NewScratchpad(WithMaxFacts(1), nil)
		require.Error(t, err)
		require.Nil(t, pad)
	})
}
