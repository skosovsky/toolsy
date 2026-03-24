package exectool

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTimeout indicates that sandbox execution exceeded its configured time budget.
	ErrTimeout = errors.New("execution timed out")
	// ErrUnsupportedLanguage indicates that the sandbox cannot execute the requested language.
	ErrUnsupportedLanguage = errors.New("unsupported language for this sandbox")
	// ErrSandboxFailure indicates an internal sandbox runtime failure unrelated to user code exit status.
	ErrSandboxFailure = errors.New("sandbox internal failure")
)

// RunRequest describes a single code execution request for a sandbox.
type RunRequest struct {
	Language string
	Code     string
	Env      map[string]string
	Files    map[string][]byte
	Timeout  time.Duration
}

// RunResult contains the observable execution outputs.
type RunResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// Sandbox executes code in an isolated or semi-isolated environment.
type Sandbox interface {
	SupportedLanguages() []string
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}
