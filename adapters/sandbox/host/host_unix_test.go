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

func TestRunKillsProcessGroupOnTimeout(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "spawn-child",
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"TOOLSY_CHILD_PID_FILE":  pidFile,
		},
		Timeout: 50 * time.Millisecond,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)

	pidBytes, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(pidBytes))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		err := syscall.Kill(pid, 0)
		return errors.Is(err, syscall.ESRCH)
	}, 2*time.Second, 25*time.Millisecond)
}
