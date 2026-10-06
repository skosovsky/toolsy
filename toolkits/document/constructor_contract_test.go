package document

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32ConstructorOptions(t *testing.T) {
	// Arrange.
	construct := func(opts ...Option) error {
		value, cleanup, err := AsToolWithCleanup(opts...)
		if err != nil {
			require.Nil(t, value)
			require.Nil(t, cleanup)
		}
		if cleanup != nil {
			cleanup()
		}
		return err
	}
	limits := []func(int) Option{
		WithMaxBytes,
		func(n int) Option { return WithLimits(Limits{SourceBytes: n}) },
		func(n int) Option { return WithLimits(Limits{ParsedBytes: n}) },
		func(n int) Option { return WithLimits(Limits{MaxItems: n}) },
		func(n int) Option { return WithLimits(Limits{ItemBytes: n}) },
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
