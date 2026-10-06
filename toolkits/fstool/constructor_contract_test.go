package fstool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD32ConstructorOptions(t *testing.T) {
	// Arrange.
	base := t.TempDir()
	construct := func(opts ...Option) error {
		value, err := AsTools(base, opts...)
		if err != nil {
			require.Nil(t, value)
		}
		return err
	}
	limits := []func(int) Option{
		WithMaxBytes,
		WithMaxSourceBytes,
		WithMaxEntries,
		WithMaxScanEntries,
		WithMaxNameBytes,
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

func TestD32InvalidOptionsBeforeBaseInspection(t *testing.T) {
	// Arrange: a missing base directory would otherwise return an os.Stat error.
	base := t.TempDir() + "/missing"
	// Act.
	tools, err := AsTools(base, nil)
	// Assert: configuration validation has precedence and no usable result.
	require.ErrorContains(t, err, "nil option")
	require.Nil(t, tools)
	tools, err = AsTools(base, WithMaxBytes(-1))
	require.ErrorContains(t, err, "invalid limits")
	require.Nil(t, tools)
	// A valid configuration still checks the actual root precondition.
	tools, err = AsTools(base)
	require.ErrorContains(t, err, "base dir does not exist")
	require.Nil(t, tools)
}
