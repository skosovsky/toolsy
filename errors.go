package toolsy

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for toolsy. Use [errors.Is] to check.
var (
	ErrToolNotFound     = errors.New("tool not found")
	ErrTimeout          = errors.New("tool execution timeout")
	ErrValidation       = errors.New("validation failed")
	ErrShutdown         = errors.New("registry is shutting down")
	ErrRegistryState    = errors.New("registry runtime state is not initialized")
	ErrStreamAborted    = errors.New("stream aborted by caller")
	ErrMaxStepsExceeded = errors.New("max execution steps exceeded")
	// ErrAsyncCollectedLimitExceeded is returned when background chunk collection exceeds WithMaxCollectedChunks.
	ErrAsyncCollectedLimitExceeded = errors.New("toolsy: async collected chunks limit exceeded")
	ErrBudgetExceeded              = errors.New("budget exceeded")
)

// ErrorCode is a machine-readable tool execution error category.
type ErrorCode string

const (
	CodeSchemaInvalid        ErrorCode = "SCHEMA_INVALID"
	CodeValidationFailed     ErrorCode = "VALIDATION_FAILED"
	CodeTimeout              ErrorCode = "TIMEOUT"
	CodeToolNotFound         ErrorCode = "TOOL_NOT_FOUND"
	CodeDependencyMissing    ErrorCode = "DEPENDENCY_MISSING"
	CodeInternal             ErrorCode = "INTERNAL"
	CodeShutdown             ErrorCode = "SHUTDOWN"
	CodeMaxStepsExceeded     ErrorCode = "MAX_STEPS_EXCEEDED"
	CodeRegistryNotReady     ErrorCode = "REGISTRY_NOT_READY"
	CodeToolsContractMissing ErrorCode = "TOOLS_CONTRACT_MISSING"
	CodeBudgetExceeded       ErrorCode = "BUDGET_EXCEEDED"
	CodeStateCodecMissing    ErrorCode = "STATE_CODEC_MISSING"
)

// ToolError is the structured execution error envelope for orchestrator routing.
// Use [AsToolError] and [errors.Is] on [Err] for sentinel checks.
type ToolError struct {
	Code        ErrorCode
	Retryable   bool
	Reason      string
	FixableArgs []string
	SafeMessage string
	Err         error
}

// NewValidationError builds a non-retryable validation [ToolError].
func NewValidationError(reason string, fixableFields ...string) *ToolError {
	return &ToolError{ //nolint:exhaustruct // SafeMessage optional for LLM-facing copy
		Code:        CodeValidationFailed,
		Reason:      reason,
		Retryable:   false,
		FixableArgs: append([]string(nil), fixableFields...),
		Err:         ErrValidation,
	}
}

// NewSchemaError builds a non-retryable schema or parse [ToolError].
func NewSchemaError(reason string) *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeSchemaInvalid,
		Reason:    reason,
		Retryable: false,
	}
}

// NewJSONParseError reports invalid tool argument JSON with details in [ToolError.Unwrap].
func NewJSONParseError(err error) *ToolError {
	if err == nil {
		return nil
	}
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeSchemaInvalid,
		Reason:    "invalid JSON",
		Retryable: false,
		Err:       err,
	}
}

// NewInternalError wraps an internal failure as [ToolError].
// Technical details are available via [ToolError.Unwrap]; [ToolError.Reason] is generic.
func NewInternalError(err error) *ToolError {
	if err == nil {
		return nil
	}
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeInternal,
		Reason:    "internal error",
		Retryable: false,
		Err:       err,
	}
}

// WithSafeMessage sets [ToolError.SafeMessage] for user-facing or LLM-safe copy.
func WithSafeMessage(te *ToolError, safe string) *ToolError {
	if te == nil {
		return nil
	}
	te.SafeMessage = safe
	return te
}

// NewDependencyMissingError reports a missing or nil typed dependency.
func NewDependencyMissingError(key string) *ToolError {
	reason := fmt.Sprintf("dependency %q is missing or nil", key)
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeDependencyMissing,
		Reason:    reason,
		Retryable: false,
		Err:       errors.New(reason),
	}
}

// NewToolNotFoundError reports an unknown tool name.
func NewToolNotFoundError() *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeToolNotFound,
		Reason:    ErrToolNotFound.Error(),
		Retryable: false,
		Err:       ErrToolNotFound,
	}
}

// NewTimeoutError reports execution timeout; set retryable when the orchestrator may retry.
func NewTimeoutError(retryable bool) *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeTimeout,
		Reason:    ErrTimeout.Error(),
		Retryable: retryable,
		Err:       ErrTimeout,
	}
}

// NewShutdownError reports registry shutdown.
func NewShutdownError() *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeShutdown,
		Reason:    ErrShutdown.Error(),
		Retryable: false,
		Err:       ErrShutdown,
	}
}

// NewMaxStepsExceededError reports session step budget exhaustion.
func NewMaxStepsExceededError() *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeMaxStepsExceeded,
		Reason:    ErrMaxStepsExceeded.Error(),
		Retryable: false,
		Err:       ErrMaxStepsExceeded,
	}
}

// NewRegistryStateError reports uninitialized registry runtime state.
func NewRegistryStateError() *ToolError {
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeRegistryNotReady,
		Reason:    ErrRegistryState.Error(),
		Retryable: false,
		Err:       ErrRegistryState,
	}
}

// NewToolsContractMissingError reports required tools missing from the registry contract.
func NewToolsContractMissingError(required, missing []string) *ToolError {
	reason := fmt.Sprintf("missing required tools: %v (contract requires %v)", missing, required)
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:        CodeToolsContractMissing,
		Reason:      reason,
		Retryable:   false,
		FixableArgs: append([]string(nil), missing...),
		Err:         errors.New(reason),
	}
}

// NewStateCodecMissingError reports a session state key without a registered codec in strict mode.
func NewStateCodecMissingError(key string) *ToolError {
	reason := fmt.Sprintf("no state codec registered for key %q", key)
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:        CodeStateCodecMissing,
		Reason:      reason,
		Retryable:   false,
		FixableArgs: []string{key},
		Err:         errors.New(reason),
	}
}

// NewSnapshotHydrationError reports a fatal snapshot import/export failure (version mismatch, corrupt payload).
func NewSnapshotHydrationError(reason string, err error) *ToolError {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "session snapshot hydration failed"
	}
	if err == nil {
		err = errors.New(reason)
	}
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeInternal,
		Reason:    reason,
		Retryable: false,
		Err:       err,
	}
}

// NewBudgetExceededError reports a token or execution budget denial.
func NewBudgetExceededError(reason string) *ToolError {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = ErrBudgetExceeded.Error()
	}
	return &ToolError{ //nolint:exhaustruct // optional envelope fields omitted by design
		Code:      CodeBudgetExceeded,
		Reason:    reason,
		Retryable: true,
		Err:       ErrBudgetExceeded,
	}
}

// NewToolNotFoundInSubsetError reports an unknown tool name when building a registry subset.
func NewToolNotFoundInSubsetError(name string) *ToolError {
	te := NewToolNotFoundError()
	te.Reason = fmt.Sprintf("unknown tool in subset: %q", name)
	return te
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Reason != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Reason)
	}
	if e.SafeMessage != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.SafeMessage)
	}
	return string(e.Code)
}

func (e *ToolError) Unwrap() error { return e.Err }

// AsToolError returns a [*ToolError] when err is or wraps one.
func AsToolError(err error) (*ToolError, bool) {
	var te *ToolError
	if errors.As(err, &te) {
		return te, true
	}
	return nil, false
}

// ClientCorrectable reports whether the error code is correctable by the caller (LLM/agent).
//
// Returns true for SCHEMA_INVALID, VALIDATION_FAILED, and TOOL_NOT_FOUND — the LLM can fix
// arguments or pick another tool. Returns false for orchestrator/host issues such as
// DEPENDENCY_MISSING, TOOLS_CONTRACT_MISSING, INTERNAL, TIMEOUT, SHUTDOWN,
// MAX_STEPS_EXCEEDED, and REGISTRY_NOT_READY; route those by comparing [ToolError.Code] explicitly.
//
// Example:
//
//	te, ok := AsToolError(err)
//	if !ok {
//	    return handleUnexpected(err)
//	}
//	switch {
//	case ClientCorrectable(te.Code):
//	    return retryWithLLMFix(te)
//	case te.Code == CodeTimeout && te.Retryable:
//	    return retryLater()
//	case te.Code == CodeToolsContractMissing:
//	    return registerMissingTools(te.FixableArgs)
//	default:
//	    return escalate(te)
//	}
func ClientCorrectable(code ErrorCode) bool {
	switch code {
	case CodeValidationFailed, CodeSchemaInvalid, CodeToolNotFound:
		return true
	default:
		return false
	}
}

func orchestratorSystemCode(code ErrorCode) bool {
	switch code {
	case CodeInternal, CodeTimeout, CodeShutdown, CodeMaxStepsExceeded, CodeRegistryNotReady, CodeStateCodecMissing:
		return true
	default:
		return false
	}
}

func clientCorrectable(err error) bool {
	te, ok := AsToolError(err)
	if !ok {
		return false
	}
	return ClientCorrectable(te.Code)
}

func wrapJSONParseError(err error) error {
	return NewJSONParseError(err)
}

// wrapYieldError wraps an error returned by the yield callback so that callers can detect
// stream abortion via [errors.Is](err, ErrStreamAborted). The original cause is preserved for Unwrap.
func wrapYieldError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrStreamAborted, err)
}
