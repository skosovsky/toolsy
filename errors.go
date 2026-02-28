package toolsy

import (
	"errors"
	"fmt"
)

// Sentinel errors for toolsy. Use errors.Is to check.
var (
	ErrToolNotFound = errors.New("tool not found")
	ErrTimeout      = errors.New("tool execution timeout")
	ErrValidation   = errors.New("validation failed")
	ErrShutdown     = errors.New("registry is shutting down")
)

// ClientError is an error that should be sent back to the LLM for self-correction
// (e.g. invalid JSON, schema validation failure, bad enum value).
// Do not expose stack traces or internal details to the LLM.
// Err optionally wraps a sentinel (e.g. ErrValidation) for errors.Is/errors.As.
type ClientError struct {
	Reason string
	// Retryable is set by the application (not by toolsy). When true, the orchestrator
	// may retry the same call without changing arguments (e.g. transient rate limit).
	Retryable bool
	Err       error // wrapped sentinel for errors.Is/errors.As
}

func (e *ClientError) Error() string {
	return fmt.Sprintf("invalid tool input: %s", e.Reason)
}

// Unwrap supports errors.Is/errors.As on wrapped chains (e.g. errors.Is(err, ErrValidation)).
func (e *ClientError) Unwrap() error { return e.Err }

// SystemError represents an internal failure (DB down, panic, etc.).
// The LLM should not see the underlying error message or stack.
type SystemError struct {
	Err error
}

func (e *SystemError) Error() string {
	return "internal system error during tool execution"
}

func (e *SystemError) Unwrap() error { return e.Err }

// IsClientError returns true if err is or wraps a ClientError.
func IsClientError(err error) bool {
	var ce *ClientError
	return errors.As(err, &ce)
}

// IsSystemError returns true if err is or wraps a SystemError.
func IsSystemError(err error) bool {
	var se *SystemError
	return errors.As(err, &se)
}

// wrapJSONParseError returns a ClientError for JSON unmarshal failures.
// Used by Extractor.ParseAndValidate and NewDynamicTool execute path so parse errors are consistent.
func wrapJSONParseError(err error) error {
	return &ClientError{Reason: "json parse error: " + err.Error()}
}
