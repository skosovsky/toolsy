//go:build !darwin && !linux

package release_test

import "os/exec"

func isolateProcess(_ *exec.Cmd) {}
