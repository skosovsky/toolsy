//go:build !darwin && !linux

package release

import (
	"context"
	"os/exec"
)

func configureCommand(_ *exec.Cmd)                 {}
func finishCommand(_ context.Context, _ *exec.Cmd) {}
