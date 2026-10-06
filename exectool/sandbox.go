package exectool

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrTimeout indicates that sandbox execution exceeded its configured time budget.
	ErrTimeout = errors.New("execution timed out")
	// ErrUnsupportedLanguage indicates that the sandbox cannot execute the requested language.
	ErrUnsupportedLanguage = errors.New("unsupported language for this sandbox")
	// ErrSandboxFailure indicates an internal sandbox runtime failure unrelated to user code exit status.
	ErrSandboxFailure = errors.New("sandbox internal failure")
	// ErrSandboxCleanup indicates that resource cleanup was not confirmed.
	ErrSandboxCleanup = errors.New("sandbox cleanup failed")
)

// CleanupError describes a secondary cleanup failure. It does not replace the
// guest's RunResult or primary interruption/execution error. Hosts can inspect
// this diagnostic with [errors.As] and reconcile remaining resources.
type CleanupError struct {
	Backend   string
	Operation string
	Cause     error
}

// Error describes the backend operation that failed during cleanup.
func (e *CleanupError) Error() string {
	return fmt.Sprintf("sandbox %s cleanup %s: %v", e.Backend, e.Operation, e.Cause)
}

// Unwrap exposes cleanup/infrastructure classification. Cause remains available
// through [errors.As] to avoid classifying a secondary cleanup deadline as an
// execution timeout or cancellation.
func (e *CleanupError) Unwrap() []error {
	return []error{ErrSandboxCleanup, ErrSandboxFailure}
}

// RunRequest describes a single code execution request for a sandbox.
type RunRequest struct {
	Language string
	Code     string
	Env      map[string]string
	Files    map[string][]byte
}

// RunResult contains complete collected guest outputs. A nonzero ExitCode is a
// guest outcome, not a Go error. Infrastructure, collection, output-limit and
// interruption failures return an error. Cleanup errors preserve this result
// when execution otherwise completed, but also return an error.
type RunResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// Sandbox executes code using backend-specific guarantees. Implementations must
// document their isolation and resource policy; the interface promises none.
type Sandbox interface {
	SupportedLanguages() []string
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}
