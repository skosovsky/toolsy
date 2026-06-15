package sandboxfs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/skosovsky/toolsy/exectool"
)

// FinalizeOrInterrupt checks context interrupts, then delegates to [FinishRun].
// Interrupt (cancel/deadline/timeout) wins over output cap errors in composite scenarios.
// Guest script failures with read-limit in stderr must pass nil runErr with exitOK=false (see FinishRun godoc).
func FinalizeOrInterrupt(
	ctx context.Context,
	runErr error,
	stdout, stderr *CappedBuffer,
	exitCode int,
	duration time.Duration,
	exitOK bool,
	trimStdoutNewline bool,
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
		if trimStdoutNewline {
			stdoutStr = strings.TrimSuffix(stdoutStr, "\n")
		}
		stdoutOverflow = stdout.OverflowErr()
	}
	if stderr != nil {
		stderrStr = stderr.String()
		stderrOverflow = stderr.OverflowErr()
	}
	return FinishRun(runErr, stdoutStr, stderrStr, exitCode, duration, exitOK, stdoutOverflow, stderrOverflow)
}
