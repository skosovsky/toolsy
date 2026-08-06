//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package mcp

import (
	"os/exec"
	"syscall"
)

func processExists(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

func newLongLivedDescendant() *exec.Cmd {
	return exec.Command("sh", "-c", "sleep 30") // #nosec G204 -- fixed test fixture.
}
