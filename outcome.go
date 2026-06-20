package toolsy

import (
	"context"
	"errors"
	"fmt"
)

// OutcomeStatus describes the terminal semantic state of RunCall.
type OutcomeStatus string

const (
	OutcomeSuccess             OutcomeStatus = "success"
	OutcomeEmptySuccess        OutcomeStatus = "empty_success"
	OutcomeNoopSuccess         OutcomeStatus = "noop_success"
	OutcomeBusinessError       OutcomeStatus = "business_error"
	OutcomeInfrastructureError OutcomeStatus = "infrastructure_error"
)

// ToolOutcome aggregates a synchronous tool call result without manual chunk glue.
type ToolOutcome struct {
	ToolName         string
	Result           []byte
	ResultMimeType   string
	TypedResult      any
	EmptyResult      bool
	Noop             bool
	Effects          []any
	Progress         []Chunk
	Controls         []ControlSignal
	CompletionPolicy CompletionPolicy
	Status           OutcomeStatus
	ExecutionError   *ToolError
}

// DecodeOutcomeAs unmarshals a successful outcome result into T.
func DecodeOutcomeAs[T any](o ToolOutcome) (*T, error) {
	if o.ExecutionError != nil {
		return nil, o.ExecutionError
	}
	if o.TypedResult != nil {
		if typed, ok := o.TypedResult.(T); ok {
			return &typed, nil
		}
		return nil, NewSchemaError(fmt.Sprintf("typed outcome result is not requested type %T", *new(T)))
	}
	if o.EmptyResult {
		return nil, NewSchemaError("cannot decode empty successful outcome")
	}
	chunk := Chunk{
		Event:    EventResult,
		Data:     o.Result,
		MimeType: o.ResultMimeType,
	}
	return DecodeChunkAs[T](chunk)
}

// DecodeOutcomeEffectsAs returns typed host effects from an outcome.
func DecodeOutcomeEffectsAs[T any](o ToolOutcome) ([]T, error) {
	if len(o.Effects) == 0 {
		return nil, nil
	}
	out := make([]T, 0, len(o.Effects))
	for _, effect := range o.Effects {
		typed, ok := effect.(T)
		if !ok {
			return nil, NewSchemaError(fmt.Sprintf("outcome effect is not requested type %T", *new(T)))
		}
		out = append(out, typed)
	}
	return out, nil
}

// RunCall executes one tool call and returns an aggregated [ToolOutcome].
// Business failures are returned as (outcome, nil) with outcome.ExecutionError set.
// Infrastructure failures return (outcome with [OutcomeInfrastructureError], err), including structured error chunks
// from [WithErrorFormatter] when the decoded [ToolError.Code] is an orchestrator/infra code.
// Control-plane signals return (outcome, err) with partial Progress/Controls preserved.
// Multiple non-error [EventResult] chunks overwrite Result (last wins).
func (s *Session) RunCall(ctx context.Context, call ToolCall) (ToolOutcome, error) {
	if s == nil {
		return newInfrastructureOutcome(""), NewValidationError("session is nil")
	}
	if s.reg == nil {
		return newInfrastructureOutcome(call.ToolName), NewToolNotFoundError()
	}

	outcome := ToolOutcome{ToolName: call.ToolName} //nolint:exhaustruct // filled during Execute
	if tool, ok := s.reg.GetTool(call.ToolName); ok {
		outcome.CompletionPolicy = tool.Manifest().CompletionPolicy
	}

	var terminalBusiness bool
	err := s.Execute(ctx, call, func(c Chunk) error {
		accountRunCallChunk(&outcome, &terminalBusiness, c)
		return nil
	})

	if err != nil {
		if IsControlError(err) {
			return outcome, err
		}
		if runCallInfraError(err) {
			outcome.Status = OutcomeInfrastructureError
			return outcome, err
		}
		outcome.ExecutionError = toolErrorFromRunCall(err)
		return finalizeRunCallOutcome(outcome)
	}
	return finalizeRunCallOutcome(outcome)
}

func newInfrastructureOutcome(toolName string) ToolOutcome {
	return ToolOutcome{ //nolint:exhaustruct // zero values intentionally mean no result/progress/effects are available.
		ToolName: toolName,
		Status:   OutcomeInfrastructureError,
	}
}

func finalizeRunCallOutcome(outcome ToolOutcome) (ToolOutcome, error) {
	if outcome.ExecutionError != nil && runCallInfraError(outcome.ExecutionError) {
		outcome.Status = OutcomeInfrastructureError
		return outcome, outcome.ExecutionError
	}
	if outcome.Status == "" {
		switch {
		case outcome.ExecutionError != nil:
			outcome.Status = OutcomeBusinessError
		case outcome.Noop:
			outcome.Status = OutcomeNoopSuccess
		case outcome.EmptyResult:
			outcome.Status = OutcomeEmptySuccess
		default:
			outcome.Status = OutcomeSuccess
		}
	}
	return outcome, nil
}

func accountRunCallChunk(outcome *ToolOutcome, terminalBusiness *bool, c Chunk) {
	switch c.Event {
	case EventProgress:
		if !*terminalBusiness {
			outcome.Progress = append(outcome.Progress, c)
		}
	case EventControl:
		if !*terminalBusiness && c.Control != nil {
			outcome.Controls = append(outcome.Controls, c.Control)
		}
	case EventResult:
		accountRunCallResult(outcome, terminalBusiness, c)
	default:
		// ignore unknown events for sync outcome aggregation
	}
}

func accountRunCallResult(outcome *ToolOutcome, terminalBusiness *bool, c Chunk) {
	if c.IsError {
		outcome.Result = nil
		outcome.ResultMimeType = ""
		outcome.TypedResult = nil
		outcome.EmptyResult = false
		outcome.Effects = nil
		outcome.ExecutionError = executionErrorFromChunk(c)
		outcome.Status = OutcomeBusinessError
		*terminalBusiness = true
		return
	}
	outcome.Result = append([]byte(nil), c.Data...)
	outcome.ResultMimeType = c.MimeType
	outcome.TypedResult = c.TypedResult
	outcome.EmptyResult = c.EmptyResult
	outcome.Noop = c.Noop
	outcome.Effects = append([]any(nil), c.Effects...)
	outcome.Controls = append(outcome.Controls, c.Controls...)
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
		CodeDependencyMissing, CodeToolsContractMissing, CodePolicyDenied, CodeCapabilityDenied:
		return true
	default:
		return false
	}
}
