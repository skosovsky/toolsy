//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package mcp

import "os/exec"

type processTree struct{}

func configureProcessTree(*exec.Cmd) {}

func attachProcessTree(*exec.Cmd) (processTree, error) { return processTree{}, nil }

func killProcessTree(_ processTree, cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
