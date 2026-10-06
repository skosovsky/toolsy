package sandboxfs

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestFinishRun_ReadLimitWithGuestExit(t *testing.T) {
	t.Parallel()
	capErr := fmt.Errorf(
		"%w: stdout exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		DefaultMaxSandboxOutputBytes,
		textprocessor.ErrReadLimitExceeded,
	)
	_, err := FinishRun(capErr, "partial", "", 7, time.Second, nil, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestFinishRun_GuestExitWithoutInfrastructureFailure(t *testing.T) {
	t.Parallel()
	res, err := FinishRun(nil, "out", "err", 7, time.Second, nil, nil)
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
	_, err := FinishRun(nil, "partial", "", 0, time.Second, overflow, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestFinishRun_CollectionFailureCannotBecomeGuestExit(t *testing.T) {
	t.Parallel()
	collectionErr := errors.New("logs unavailable")

	result, err := FinishRun(collectionErr, "partial", "", 7, time.Second, nil, nil)

	require.ErrorIs(t, err, collectionErr)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.Empty(t, result)
}

func TestWithCleanupError_PreservesPrimaryAndDiagnostic(t *testing.T) {
	t.Parallel()
	primary := exectool.ErrTimeout
	cleanupErr := errors.New("remove refused")

	err := WithCleanupError(primary, "test", "remove", cleanupErr)

	require.ErrorIs(t, err, primary)
	require.NotErrorIs(t, err, cleanupErr)
	require.ErrorIs(t, err, exectool.ErrSandboxCleanup)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	var diagnostic *exectool.CleanupError
	require.ErrorAs(t, err, &diagnostic)
	require.Equal(t, "test", diagnostic.Backend)
	require.Equal(t, "remove", diagnostic.Operation)
	require.Same(t, cleanupErr, diagnostic.Cause)
}

func TestWithCleanupError_CompletedAndSuccessfulCleanup(t *testing.T) {
	t.Parallel()
	cleanupErr := errors.New("remove refused")

	require.ErrorIs(t, WithCleanupError(nil, "test", "remove", cleanupErr), exectool.ErrSandboxCleanup)
	require.NoError(t, WithCleanupError(nil, "test", "remove", nil))
	require.Same(t, exectool.ErrTimeout, WithCleanupError(exectool.ErrTimeout, "test", "remove", nil))
}

func TestWithCleanupError_SecondaryCancellationCannotChangePrimary(t *testing.T) {
	t.Parallel()
	err := WithCleanupError(exectool.ErrTimeout, "test", "remove", context.Canceled)

	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.NotErrorIs(t, err, context.Canceled)
	var diagnostic *exectool.CleanupError
	require.ErrorAs(t, err, &diagnostic)
	require.ErrorIs(t, diagnostic.Cause, context.Canceled)
	completed := WithCleanupError(nil, "test", "remove", context.DeadlineExceeded)
	require.NotErrorIs(t, completed, context.DeadlineExceeded)
	require.ErrorIs(t, completed, exectool.ErrSandboxFailure)
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
