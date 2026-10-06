package sandboxfs

import (
	"context"
	"errors"
	"time"

	"github.com/skosovsky/toolsy/exectool"
)

// FinalizeOrInterrupt checks context interrupts, then delegates to [FinishRun].
// Interrupt (cancel/deadline/timeout) wins over output cap errors in composite scenarios.
// Known guest exits pass nil runErr with their exitCode (see FinishRun godoc).
func FinalizeOrInterrupt(
	ctx context.Context,
	runErr error,
	stdout, stderr *CappedBuffer,
	exitCode int,
	duration time.Duration,
) (exectool.RunResult, error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return exectool.RunResult{}, exectool.ErrTimeout
	}
	if ctx.Err() != nil {
		return exectool.RunResult{}, ctx.Err()
	}
	stdoutStr, stderrStr := "", ""
	var stdoutOverflow, stderrOverflow error
	if stdout != nil {
		stdoutStr = stdout.String()
		stdoutOverflow = stdout.OverflowErr()
	}
	if stderr != nil {
		stderrStr = stderr.String()
		stderrOverflow = stderr.OverflowErr()
	}
	return FinishRun(runErr, stdoutStr, stderrStr, exitCode, duration, stdoutOverflow, stderrOverflow)
}
