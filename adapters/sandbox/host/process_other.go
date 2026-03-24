//go:build !unix

package host

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
