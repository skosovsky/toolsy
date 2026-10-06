package sandboxfs

import (
	"errors"
	"fmt"
	"time"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/textprocessor"
)

// FinishRun builds a [exectool.RunResult] or returns an error when output collection failed (e.g. cap exceeded).
// Callers normalize known guest exits to nil runErr and their exitCode.
// Every non-nil runErr denotes incomplete execution or output collection, never a guest exit.
// stdoutOverflow/stderrOverflow capture [CappedBuffer] overflow even when the process exits non-zero.
func FinishRun(
	runErr error,
	stdout, stderr string,
	exitCode int,
	duration time.Duration,
	stdoutOverflow, stderrOverflow error,
) (exectool.RunResult, error) {
	if failure := errors.Join(runErr, stdoutOverflow, stderrOverflow); failure != nil {
		return exectool.RunResult{}, fmt.Errorf("%w: execute: %w", exectool.ErrSandboxFailure, failure)
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
