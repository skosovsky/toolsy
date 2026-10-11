package toolsy

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrInputAdmissionUnsupported identifies tools without a separate input boundary,
// including opaque custom wrappers and invalid or excessive native wrapper chains.
var ErrInputAdmissionUnsupported = errors.New("toolsy: unsupported input admission")

// InputAdmitter optionally checks input without dispatching a tool. Implementations
// must use the same input transformation and validation as Execute, return a
// defensive canonical snapshot, and never treat success as authorization.
// User callbacks must be repeatable and avoid business effects; their purity is
// host-owned. Panics propagate to the caller, including through WithRecovery.
type InputAdmitter interface {
	AdmitInput(ctx context.Context, call CallContext, in ToolInput) (ToolInput, error)
}

const maxInputAdmissionLayers = 64

// AdmitInput checks typed/dynamic constructor inputs through native wrappers.
// It never calls Execute, policy, authorization, profiles, controls or budgets.
// Only known native execution wrappers are transparent; ChainUnwrapper alone
// does not authorize bypassing a custom wrapper. Custom tools must implement
// InputAdmitter, including any transformation performed by their Execute method.
// Success returns canonical input, not a PreparedCall or an execution grant.
// Dispatch must bind and authorize again with its current context and arguments.
// Callback panics propagate; this boundary does not invoke Registry recovery.
func AdmitInput(ctx context.Context, t Tool, call CallContext, in ToolInput) (ToolInput, error) {
	if err := ctx.Err(); err != nil {
		return ToolInput{}, normalizeExecutionInterrupt(err)
	}
	for range maxInputAdmissionLayers {
		if isNilValue(t) {
			return ToolInput{}, ErrInputAdmissionUnsupported
		}
		switch t.(type) {
		case *middlewareTool,
			*recoveryTool,
			*budgetTool,
			*truncationTool,
			*errorFormatterTool,
			*asyncTool,
			*overriddenTool:
			wrapper, ok := t.(ChainUnwrapper)
			if !ok {
				return ToolInput{}, ErrInputAdmissionUnsupported
			}
			t = wrapper.UnwrapNext()
		default:
			admitter, ok := t.(InputAdmitter)
			if !ok {
				return ToolInput{}, ErrInputAdmissionUnsupported
			}
			out, err := admitter.AdmitInput(ctx, preparedContextSnapshot(call), in.Clone())
			if err != nil {
				return ToolInput{}, err
			}
			if canceled := ctx.Err(); canceled != nil {
				return ToolInput{}, normalizeExecutionInterrupt(canceled)
			}
			return out.Clone(), nil
		}
	}
	return ToolInput{}, ErrInputAdmissionUnsupported
}

// AdmitInput implements the optional input boundary for supported constructors.
func (t *tool) AdmitInput(ctx context.Context, call CallContext, in ToolInput) (ToolInput, error) {
	if err := ctx.Err(); err != nil {
		return ToolInput{}, normalizeExecutionInterrupt(err)
	}
	if t == nil || t.admit == nil {
		return ToolInput{}, ErrInputAdmissionUnsupported
	}
	input := in.Clone()
	value, err := t.admit(ctx, preparedContextSnapshot(call), input)
	if err != nil {
		return ToolInput{}, err
	}
	if canceled := ctx.Err(); canceled != nil {
		return ToolInput{}, normalizeExecutionInterrupt(canceled)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ToolInput{}, NewInternalError(err)
	}
	input.ArgsJSON = raw
	return input.Clone(), nil
}
