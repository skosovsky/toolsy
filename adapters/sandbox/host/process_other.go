//go:build !unix

package host

import (
	"context"
	"errors"
	"os/exec"
)

func newRuntimeCommand(ctx context.Context, command string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, command, args...)
}

func runRuntime(_ context.Context, cmd *exec.Cmd) (int, error, error) {
	err := cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), nil, nil
	}
	return 0, err, nil
}
