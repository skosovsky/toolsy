package sandboxfs

import (
	"fmt"
	"time"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

// FinishRun builds a [exectool.RunResult] or returns an error when output collection failed (e.g. cap exceeded).
// When exitOK is true, a non-nil runErr is treated as a non-zero process exit, not an infrastructure failure.
// Guest script failures (e.g. starlark eval) that surface read-limit in stderr must pass nil runErr with exitOK=false.
// stdoutOverflow/stderrOverflow capture [CappedBuffer] overflow even when the process exits non-zero.
func FinishRun(
	runErr error,
	stdout, stderr string,
	exitCode int,
	duration time.Duration,
	exitOK bool,
	stdoutOverflow, stderrOverflow error,
) (exectool.RunResult, error) {
	if textprocessor.IsReadLimitExceeded(runErr) {
		return exectool.RunResult{}, fmt.Errorf("%w: execute: %w", exectool.ErrSandboxFailure, runErr)
	}
	if stdoutOverflow != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: execute: %w", exectool.ErrSandboxFailure, stdoutOverflow)
	}
	if stderrOverflow != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: execute: %w", exectool.ErrSandboxFailure, stderrOverflow)
	}
	if exitOK {
		return exectool.RunResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Duration: duration,
		}, nil
	}
	if runErr != nil {
		return exectool.RunResult{}, runErr
	}
	return exectool.RunResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

// ReadLimitSubject extracts the capped stream name (e.g. "stdout") from a sandbox read-limit error chain.
func ReadLimitSubject(err error) string {
	return textprocessor.ReadLimitSubject(err)
}

// ReadLimitMaxBytes extracts the byte cap from a sandbox read-limit error chain.
func ReadLimitMaxBytes(err error) int {
	return textprocessor.ReadLimitMaxBytes(err)
}
