package toolsy

import (
	"encoding/json"
	"errors"
)

// MimeTypeToolErrorJSON carries a structured [ToolError] envelope in error result chunks.
const MimeTypeToolErrorJSON = "application/vnd.toolsy.tool-error+json"

type toolErrorWire struct {
	Code        ErrorCode `json:"code"`
	Retryable   bool      `json:"retryable"`
	Reason      string    `json:"reason,omitempty"`
	FixableArgs []string  `json:"fixable_args,omitempty"`
	SafeMessage string    `json:"safe_message,omitempty"`
	Message     string    `json:"message,omitempty"`
}

func marshalToolErrorWire(te *ToolError, llmMessage string) ([]byte, error) {
	if te == nil {
		return nil, errors.New("toolsy: nil ToolError")
	}
	wire := toolErrorWire{
		Code:        te.Code,
		Retryable:   te.Retryable,
		Reason:      te.Reason,
		FixableArgs: append([]string(nil), te.FixableArgs...),
		SafeMessage: te.SafeMessage,
		Message:     llmMessage,
	}
	return json.Marshal(wire)
}

func unmarshalToolErrorWire(data []byte) (*ToolError, error) {
	var wire toolErrorWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire.Code == "" {
		return nil, errors.New("toolsy: tool error wire missing code")
	}
	te := &ToolError{ //nolint:exhaustruct // Err restored below when possible
		Code:        wire.Code,
		Retryable:   wire.Retryable,
		Reason:      wire.Reason,
		FixableArgs: append([]string(nil), wire.FixableArgs...),
		SafeMessage: wire.SafeMessage,
	}
	te.Err = sentinelForErrorCode(wire.Code)
	if te.Err == nil {
		switch {
		case wire.Message != "":
			te.Err = errors.New(wire.Message)
		case wire.Reason != "":
			te.Err = errors.New(wire.Reason)
		}
	}
	return te, nil
}

func sentinelForErrorCode(code ErrorCode) error {
	switch code {
	case CodeValidationFailed:
		return ErrValidation
	case CodeToolNotFound:
		return ErrToolNotFound
	case CodeTimeout:
		return ErrTimeout
	case CodeShutdown:
		return ErrShutdown
	case CodeMaxStepsExceeded:
		return ErrMaxStepsExceeded
	case CodeRegistryNotReady:
		return ErrRegistryState
	case CodeBudgetExceeded:
		return ErrBudgetExceeded
	case CodeSchemaInvalid:
		return ErrValidation
	case CodeDependencyMissing, CodeToolsContractMissing:
		return nil
	default:
		return nil
	}
}
