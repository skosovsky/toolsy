package sandboxfs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestFinalizeOrInterrupt_TimeoutOverOverflow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	stdout := NewCappedBuffer("stdout", 10)
	_, _ = stdout.Write([]byte(strings.Repeat("x", 11)))

	_, err := FinalizeOrInterrupt(ctx, nil, stdout, nil, 0, 0, true, false)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestFinalizeOrInterrupt_NonExitErrorWithOverflow(t *testing.T) {
	t.Parallel()
	stdout := NewCappedBuffer("stdout", 10)
	_, _ = stdout.Write([]byte(strings.Repeat("x", 11)))
	infra := fmt.Errorf("%w: execute runtime: %w", exectool.ErrSandboxFailure, errors.New("boom"))

	_, err := FinalizeOrInterrupt(context.Background(), infra, stdout, nil, 0, 0, false, false)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestFinalizeOrInterrupt_CancelOverOverflow(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout := NewCappedBuffer("stdout", 10)
	_, _ = stdout.Write([]byte(strings.Repeat("x", 11)))

	_, err := FinalizeOrInterrupt(ctx, nil, stdout, nil, 0, 0, true, false)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
