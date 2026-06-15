package toolsy

import (
	"context"
	"errors"
	"fmt"
)

// ToolOutcome aggregates a synchronous tool call result without manual chunk glue.
type ToolOutcome struct {
	ToolName         string
	Result           []byte
	ResultMimeType   string
	Progress         []Chunk
	Controls         []ControlSignal
	CompletionPolicy CompletionPolicy
	ExecutionError   *ToolError
}

// DecodeOutcomeAs unmarshals a successful outcome result into T.
func DecodeOutcomeAs[T any](o ToolOutcome) (*T, error) {
	if o.ExecutionError != nil {
		return nil, o.ExecutionError
	}
	chunk := Chunk{
		Event:    EventResult,
		Data:     o.Result,
		MimeType: o.ResultMimeType,
	}
	return DecodeChunkAs[T](chunk)
}

// RunCall executes one tool call and returns an aggregated [ToolOutcome].
// Business failures are returned as (outcome, nil) with outcome.ExecutionError set.
// Infrastructure failures return (zero ToolOutcome, err), including structured error chunks
// from [WithErrorFormatter] when the decoded [ToolError.Code] is an orchestrator/infra code.
// Control-plane signals return (outcome, err) with partial Progress/Controls preserved.
// Multiple non-error [EventResult] chunks overwrite Result (last wins).
func (s *Session) RunCall(ctx context.Context, call ToolCall) (ToolOutcome, error) {
	if s == nil {
		return ToolOutcome{}, NewValidationError("session is nil")
	}
	if s.reg == nil {
		return ToolOutcome{}, NewToolNotFoundError()
	}

	outcome := ToolOutcome{ToolName: call.ToolName} //nolint:exhaustruct // filled during Execute
	if tool, ok := s.reg.GetTool(call.ToolName); ok {
		outcome.CompletionPolicy = tool.Manifest().CompletionPolicy
	}

	var terminalBusiness bool
	err := s.Execute(ctx, call, func(c Chunk) error {
		switch c.Event {
		case EventProgress:
			if !terminalBusiness {
				outcome.Progress = append(outcome.Progress, c)
			}
		case EventControl:
			if !terminalBusiness && c.Control != nil {
				outcome.Controls = append(outcome.Controls, c.Control)
			}
		case EventResult:
			if c.IsError {
				outcome.Result = nil
				outcome.ResultMimeType = ""
				outcome.ExecutionError = executionErrorFromChunk(c)
				terminalBusiness = true
				return nil
			}
			outcome.Result = append([]byte(nil), c.Data...)
			outcome.ResultMimeType = c.MimeType
		default:
			// ignore unknown events for sync outcome aggregation
		}
		return nil
	})

	if err != nil {
		if IsControlError(err) {
			return outcome, err
		}
		if runCallInfraError(err) {
			return ToolOutcome{}, err
		}
		outcome.ExecutionError = toolErrorFromRunCall(err)
		return finalizeRunCallOutcome(outcome)
	}
	return finalizeRunCallOutcome(outcome)
}

func finalizeRunCallOutcome(outcome ToolOutcome) (ToolOutcome, error) {
	if outcome.ExecutionError != nil && runCallInfraError(outcome.ExecutionError) {
		return ToolOutcome{}, outcome.ExecutionError
	}
	return outcome, nil
}

func executionErrorFromChunk(c Chunk) *ToolError {
	if c.IsError {
		c = normalizeErrorChunk(c)
	}
	if c.MimeType == MimeTypeToolErrorJSON {
		te, err := unmarshalToolErrorWire(c.Data)
		if err != nil {
			return NewInternalError(fmt.Errorf("toolsy: corrupt tool error envelope: %w", err))
		}
		return te
	}
	return NewInternalError(errors.New("tool returned empty error chunk"))
}

func toolErrorFromRunCall(err error) *ToolError {
	if isContextInterrupt(err) {
		return nil
	}
	if te, ok := AsToolError(err); ok {
		return te
	}
	return NewInternalError(err)
}

func runCallInfraError(err error) bool {
	if err == nil {
		return false
	}
	if isContextInterrupt(err) {
		return true
	}
	if errors.Is(err, ErrStreamAborted) {
		return true
	}
	te, ok := AsToolError(err)
	if !ok {
		return true
	}
	if orchestratorSystemCode(te.Code) {
		return true
	}
	switch te.Code {
	case CodeToolNotFound, CodeShutdown, CodeMaxStepsExceeded, CodeRegistryNotReady,
		CodeDependencyMissing, CodeToolsContractMissing:
		return true
	default:
		return false
	}
}
