//go:build unix

package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
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

func TestRunStopsDescendantsAfterGuestCompletion(t *testing.T) {
	for _, exit := range []int{0, 7} {
		for _, redirected := range []bool{false, true} {
			t.Run(fmt.Sprintf("exit%d/redirected%t", exit, redirected), func(t *testing.T) {
				// Arrange: child remains in the owned group, with either inherited or redirected pipes.
				root := t.TempDir()
				pidFile := filepath.Join(t.TempDir(), "child.pid")
				t.Cleanup(func() {
					data, _ := os.ReadFile(pidFile)
					pid, _ := strconv.Atoi(string(data))
					if pid > 0 {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				})
				sb, err := New(
					WithRuntime("sh", Runtime{Command: "/bin/sh", ScriptName: "main.sh"}),
					WithTempDirRoot(root),
				)
				require.NoError(t, err)
				redirect := ""
				if redirected {
					redirect = " >/dev/null 2>&1"
				}
				code := fmt.Sprintf(
					"/bin/sleep 30%s &\nchild=$!\nprintf '%%s' \"$child\" > \"$PIDFILE\"\nprintf guest\nexit %d\n",
					redirect,
					exit,
				)
				// Act.
				result, runErr := sb.Run(
					t.Context(),
					exectool.RunRequest{Language: "sh", Code: code, Env: map[string]string{"PIDFILE": pidFile}},
				)
				// Assert.
				require.NoError(t, runErr)
				require.Equal(t, exit, result.ExitCode)
				require.Equal(t, "guest", result.Stdout)
				data, err := os.ReadFile(pidFile)
				require.NoError(t, err)
				pid, err := strconv.Atoi(string(data))
				require.NoError(t, err)
				require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		}
	}
}

func TestFailedGroupSignalStopsOwnedDirectProcesses(t *testing.T) {
	// Arrange: inject a group-level failure while direct owned processes remain killable.
	anchor := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	var attr syscall.SysProcAttr
	attr.Setpgid = true
	anchor.SysProcAttr = &attr
	require.NoError(t, anchor.Start())
	t.Cleanup(func() { _ = anchor.Process.Kill(); _ = anchor.Wait() })
	guest := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	var guestAttr syscall.SysProcAttr
	guestAttr.Setpgid = true
	guestAttr.Pgid = anchor.Process.Pid
	guest.SysProcAttr = &guestAttr
	require.NoError(t, guest.Start())
	t.Cleanup(func() { _ = guest.Process.Kill(); _ = guest.Wait() })
	refused := errors.New("group signal refused")
	// Act.
	stopErr := stopOwnedWork(anchor, guest, func(*exec.Cmd) error { return refused })
	guestErr := guest.Wait()
	anchorErr := anchor.Wait()
	// Assert: fallback kills both direct children, but never hides failed group cleanup.
	require.ErrorIs(t, stopErr, refused)
	require.Error(t, guestErr)
	require.True(t, expectedAnchorStop(anchorErr))
}

func TestOutputOverflowStopsDescendantsAndRemovesWorkspace(t *testing.T) {
	// Arrange: collection failure and normal completion can request the same stop.
	root := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Cleanup(func() {
		data, _ := os.ReadFile(pidFile)
		pid, _ := strconv.Atoi(string(data))
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	sb, err := New(WithRuntime("sh", Runtime{Command: "/bin/sh", ScriptName: "main.sh"}), WithTempDirRoot(root))
	require.NoError(t, err)
	code := `/bin/sleep 30 &
printf '%s' "$!" > "$PIDFILE"
i=0; while [ $i -lt 30000 ]; do printf 0123456789; i=$((i+1)); done`
	// Act.
	_, err = sb.Run(
		t.Context(),
		exectool.RunRequest{Language: "sh", Code: code, Env: map[string]string{"PIDFILE": pidFile}},
	)
	// Assert.
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.NotErrorIs(t, err, exectool.ErrSandboxCleanup)
	data, readErr := os.ReadFile(pidFile)
	require.NoError(t, readErr)
	pid, parseErr := strconv.Atoi(string(data))
	require.NoError(t, parseErr)
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
	entries, readErr := os.ReadDir(root)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}
