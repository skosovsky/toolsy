//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package mcp

import (
	"errors"
	"os/exec"
	"syscall"
)

type processTree struct {
	processGroupID int
}

func configureProcessTree(cmd *exec.Cmd) {
	//nolint:exhaustruct_v5 // The zero values of all other process attributes are intentional.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachProcessTree(cmd *exec.Cmd) (processTree, error) {
	if cmd.Process == nil {
		return processTree{}, errors.New("process was not started")
	}
	return processTree{processGroupID: cmd.Process.Pid}, nil
}

func killProcessTree(tree processTree, cmd *exec.Cmd) error {
	if tree.processGroupID > 0 {
		err := syscall.Kill(-tree.processGroupID, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
	}
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
