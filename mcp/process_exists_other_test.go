//go:build !windows && !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package mcp

import "os/exec"

func processExists(int) bool { return false }

func newLongLivedDescendant() *exec.Cmd { return nil }
