package toolsy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestMapToolkitReadError_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	composite := fmt.Errorf("read: %w", textprocessor.ErrReadLimitExceeded)
	mapped := MapToolkitReadError(ctx, composite, "toolkit/test: read", 4096, "body", "")
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	_, ok := AsToolError(mapped)
	require.True(t, ok)
}

func TestMapToolkitReadError_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	mapped := MapToolkitReadError(
		context.Background(),
		composite,
		"toolkit/test: read",
		4096,
		"body",
		"",
	)
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	te, ok := AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, CodeInternal, te.Code)
	require.NotEqual(t, CodeValidationFailed, te.Code)
}

func TestMapToolkitReadError_ReadLimit(t *testing.T) {
	t.Parallel()
	mapped := MapToolkitReadError(
		context.Background(),
		textprocessor.ErrReadLimitExceeded,
		"toolkit/test: read",
		4096,
		"body",
		"",
	)
	te, ok := AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)
}

func TestToolkitContextError_Canceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ToolkitContextError(ctx, "toolkit/test: stat")
	require.ErrorIs(t, err, context.Canceled)
}

func TestMapToolkitCapError_CancelOverLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mapped := MapToolkitCapError(ctx, "toolkit/test: cap", 4096, "table", "")
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	_, ok := AsToolError(mapped)
	require.True(t, ok)
}

func TestMapToolkitCapError_ReadLimit(t *testing.T) {
	t.Parallel()
	mapped := MapToolkitCapError(context.Background(), "toolkit/test: cap", 4096, "table", "")
	te, ok := AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "table exceeds 4096 byte limit")
}
