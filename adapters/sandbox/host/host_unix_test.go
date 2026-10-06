//go:build unix

package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

func TestRunKillsProcessGroupOnCancellation(t *testing.T) {
	// Arrange.
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	// Act: wait for actual child readiness before cancellation, not a startup timing guess.
	go func() {
		_, runErr := sb.Run(ctx, exectool.RunRequest{
			Language: "helper", Code: "spawn-child",
			Env: map[string]string{"GO_WANT_HELPER_PROCESS": "1", "TOOLSY_CHILD_PID_FILE": pidFile},
		})
		finished <- runErr
	}()
	require.Eventually(
		t,
		func() bool {
			data, readErr := os.ReadFile(pidFile)
			if readErr != nil {
				return false
			}
			childPID, parseErr := strconv.Atoi(string(data))
			return parseErr == nil && childPID > 0
		},
		3*time.Second,
		10*time.Millisecond,
	)
	pidBytes, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(pidBytes))
	require.NoError(t, err)
	cancel()
	// Assert.
	select {
	case err = <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not terminate after cancellation")
	}
	require.Eventually(
		t,
		func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) },
		2*time.Second,
		25*time.Millisecond,
	)
}

func TestRunReportsWorkspaceCleanupFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission-based cleanup failure control")
	}
	for _, code := range []string{"cleanup-failure", "cleanup-failure-exit"} {
		t.Run(code, func(t *testing.T) {
			// Arrange.
			parent := t.TempDir()
			t.Cleanup(func() {
				entries, _ := os.ReadDir(parent)
				for _, entry := range entries {
					_ = os.Chmod(filepath.Join(parent, entry.Name()), 0o700)
				}
			})
			sb, err := New(WithRuntime("helper", helperRuntime()), WithTempDirRoot(parent))
			require.NoError(t, err)
			// Act.
			result, err := sb.Run(
				context.Background(),
				exectool.RunRequest{
					Language: "helper",
					Code:     code,
					Env:      map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
				},
			)
			// Assert.
			require.ErrorIs(t, err, exectool.ErrSandboxCleanup)
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
			require.Equal(t, "finished", result.Stdout)
			if code == "cleanup-failure-exit" {
				require.Equal(t, 7, result.ExitCode)
			} else {
				require.Zero(t, result.ExitCode)
			}
			var diagnostic *exectool.CleanupError
			require.ErrorAs(t, err, &diagnostic)
			require.Equal(t, "host", diagnostic.Backend)
		})
	}
}
