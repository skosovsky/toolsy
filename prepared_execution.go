package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// PreparedCall is a defensive snapshot after argument binding and authorization.
// Input contains canonical arguments; Context and View remain host-owned contracts.
type PreparedCall struct {
	Manifest ToolManifest
	Input    ToolInput
	Context  CallContext
	View     RegistryViewSnapshot
}

// InvocationHandler dispatches an already authorized invocation. It must not be
// retried by a profile after an unknown external outcome.
type InvocationHandler func(func(Chunk) error) error

// ExecutionProfile controls dispatch or replay after current binding and policy.
// A profile must treat its handler as a single attempt, not a retry loop.
type ExecutionProfile interface {
	ExecutePrepared(context.Context, PreparedCall, InvocationHandler, func(Chunk) error) error
}

// PreparedExecutionTool advertises an execution boundary preserved by a tool chain.
// Custom implementations must call ExecutePrepared after all binding and policy
// checks and before every handler invocation.
type PreparedExecutionTool interface {
	SupportsPreparedExecution() bool
}

// WithExecutionProfile installs a prepared execution profile for a registry.
func WithExecutionProfile(profile ExecutionProfile) RegistryOption {
	return func(o *registryOptions) { o.executionProfile = profile }
}

// WithRunExecutionProfile installs a profile for direct Tool.Execute calls.
func WithRunExecutionProfile(profile ExecutionProfile) RunEnvOption {
	return func(env *RunEnv) { env.executionProfile = profile }
}

// WithRunCallContext binds trusted host identity to a direct tool invocation.
// This does not authenticate model data or replace Registry/Session policy and
// budgets. Adapters must use their supplied scoped executor, not direct dispatch.
func WithRunCallContext(call CallContext) RunEnvOption {
	return func(env *RunEnv) { env.callContext = preparedContextSnapshot(call) }
}

// ExecutePrepared is the boundary used by custom tools after their binding and
// authorization. The continuation receives a private copy of canonical input.
func ExecutePrepared(
	ctx context.Context,
	env *RunEnv,
	manifest ToolManifest,
	input ToolInput,
	canonicalArgs any,
	handler func(ToolInput, func(Chunk) error) error,
	yield func(Chunk) error,
) error {
	if err := ctx.Err(); err != nil {
		return normalizeExecutionInterrupt(err)
	}
	if env != nil && env.preparedDispatch {
		return NewValidationError("direct nested execution requires a fresh RunEnv or scoped executor")
	}
	if env == nil || (env.executionProfile == nil && len(env.preparedChecks) == 0) {
		return handler(input.Clone(), yield)
	}
	raw, err := json.Marshal(canonicalArgs)
	if err != nil {
		return NewInternalError(err)
	}
	bound := input.Clone()
	bound.ArgsJSON = raw
	if env.executionManifest != nil {
		manifest = *env.executionManifest
	}
	call := PreparedCall{
		Manifest: cloneManifestForPolicy(manifest),
		Input:    bound.Clone(),
		Context:  preparedContextSnapshot(env.CallContext()),
		View:     env.RegistryViewSnapshot(),
	}
	for _, check := range env.preparedChecks {
		if err := check(ctx, clonePreparedCall(call), deepCloneValue(canonicalArgs)); err != nil {
			return err
		}
	}
	if env.executionProfile == nil {
		env.preparedDispatch = true
		return handler(bound.Clone(), yield)
	}
	var dispatched atomic.Bool
	return env.executionProfile.ExecutePrepared(ctx, call, func(nextYield func(Chunk) error) error {
		if !dispatched.CompareAndSwap(false, true) {
			return NewValidationError("prepared invocation cannot dispatch twice")
		}
		env.preparedDispatch = true
		return handler(bound.Clone(), nextYield)
	}, yield)
}

func supportsPreparedExecution(t Tool) bool {
	prepared, ok := t.(PreparedExecutionTool)
	return ok && prepared.SupportsPreparedExecution()
}

func validatePreparedTool(t Tool, profile ExecutionProfile) error {
	if profile != nil && !supportsPreparedExecution(t) {
		return fmt.Errorf("toolsy: tool %q has no prepared execution boundary", t.Manifest().Name)
	}
	return nil
}

func clonePreparedCall(call PreparedCall) PreparedCall {
	call.Manifest = cloneManifestForPolicy(call.Manifest)
	call.Input = call.Input.Clone()
	call.Context = preparedContextSnapshot(call.Context)
	call.View = cloneRegistryViewSnapshot(call.View)
	return call
}

func preparedContextSnapshot(call CallContext) CallContext {
	call.Metadata = cloneCallMetadata(call.Metadata)
	call.Subject = deepCloneValue(call.Subject)
	call.Scope = deepCloneValue(call.Scope)
	call.Values = deepCloneMap(call.Values)
	return call
}

// SupportsPreparedExecution reports that built-in tools use the prepared boundary.
func (t *tool) SupportsPreparedExecution() bool { return true }

// SupportsPreparedExecution propagates the boundary through standard wrappers.
func (b *toolBase) SupportsPreparedExecution() bool { return supportsPreparedExecution(b.next) }
