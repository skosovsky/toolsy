//go:build unix

package host

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) {
	var attr syscall.SysProcAttr
	attr.Setpgid = true
	cmd.SysProcAttr = &attr
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill process group: %w", err)
		}
		return nil
	}
}
