package sandboxfs

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestFinishRun_ReadLimitBeforeExitOK(t *testing.T) {
	t.Parallel()
	capErr := fmt.Errorf(
		"%w: stdout exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		DefaultMaxSandboxOutputBytes,
		textprocessor.ErrReadLimitExceeded,
	)
	_, err := FinishRun(capErr, "partial", "", 7, time.Second, true, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestFinishRun_ExitOKWithoutReadLimit(t *testing.T) {
	t.Parallel()
	res, err := FinishRun(errors.New("exit status 7"), "out", "err", 7, time.Second, true, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 7, res.ExitCode)
	require.Equal(t, "out", res.Stdout)
}

func TestFinishRun_StdoutOverflowExitZero(t *testing.T) {
	t.Parallel()
	overflow := fmt.Errorf(
		"%w: stdout exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		4096,
		textprocessor.ErrReadLimitExceeded,
	)
	_, err := FinishRun(nil, "partial", "", 0, time.Second, true, overflow, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestReadLimitSubjectAndMaxBytes(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf(
		"%w: container stdout exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		4096,
		textprocessor.ErrReadLimitExceeded,
	)
	require.Equal(t, "container stdout", ReadLimitSubject(err))
	require.Equal(t, 4096, ReadLimitMaxBytes(err))
}
