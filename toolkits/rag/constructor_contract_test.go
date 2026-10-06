package rag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32ConstructorOptions(t *testing.T) {
	// Arrange.
	construct := func(opts ...Option) error {
		value, err := AsSearchTool(&mockRetriever{}, opts...)
		if err != nil {
			require.Nil(t, value)
		}
		return err
	}
	limits := []func(int) Option{
		WithMaxBytes,
		WithMaxResults,
		WithMaxItemBytes,
		WithMaxSourceBytes,
	}
	// Act and Assert: a nil option must fail with an error, never a panic.
	t.Run("nil", func(t *testing.T) { require.NotPanics(t, func() { require.Error(t, construct(nil)) }) })
	for i, option := range limits {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			// Arrange.
			negative, zero := option(-1), option(0)
			// Act.
			err := construct(negative)
			// Assert.
			require.Error(t, err)
			require.NoError(t, construct(zero))
			// Last option wins; earlier invalid value is replaced before validation.
			require.NoError(t, construct(negative, option(1024)))
			require.Error(t, construct(option(1024), negative))
		})
	}
}
