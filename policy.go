package toolsy

import (
	"context"
	"errors"
	"fmt"
)

// ErrPolicyDenied is the sentinel for fail-closed policy/capability denial.
var ErrPolicyDenied = errors.New("policy denied")

// ErrCapabilityDenied is the sentinel for view/capability denial.
var ErrCapabilityDenied = errors.New("capability denied")

// Decision is a machine-readable allow/deny result from policy/capability checks.
type Decision struct {
	Allow       bool
	Code        ErrorCode
	Reason      string
	SafeMessage string
	FixableArgs []string
	Retryable   bool
	Err         error
}

// AllowDecision explicitly permits a call.
func AllowDecision() Decision {
	return Decision{
		Allow:       true,
		Code:        "",
		Reason:      "",
		SafeMessage: "",
		FixableArgs: nil,
		Retryable:   false,
		Err:         nil,
	}
}

// DenyDecision rejects a call with a structured policy error.
func DenyDecision(reason string, fixableArgs ...string) Decision {
	return Decision{
		Allow:       false,
		Code:        CodePolicyDenied,
		Reason:      reason,
		SafeMessage: "",
		FixableArgs: append([]string(nil), fixableArgs...),
		Retryable:   false,
		Err:         ErrPolicyDenied,
	}
}

// PolicyRequest is the type-erased request evaluated before tool execution.
type PolicyRequest struct {
	Manifest    ToolManifest
	Input       ToolInput
	CallContext CallContext
	View        RegistryViewSnapshot
}

// Policy performs universal fail-closed call authorization.
type Policy interface {
	Decide(ctx context.Context, req PolicyRequest) Decision
}

// PolicyFunc adapts a function to Policy.
type PolicyFunc func(context.Context, PolicyRequest) Decision

// Decide implements Policy.
func (f PolicyFunc) Decide(ctx context.Context, req PolicyRequest) Decision {
	if f == nil {
		return DenyDecision("policy function is nil")
	}
	return f(ctx, req)
}

// AuthorizationRequest is the request passed to legacy-compatible authorizers.
type AuthorizationRequest = PolicyRequest

// NewPolicyDeniedError creates a structured ToolError for policy/capability denial.
func NewPolicyDeniedError(reason string, fixableArgs ...string) *ToolError {
	if reason == "" {
		reason = ErrPolicyDenied.Error()
	}
	return &ToolError{
		Code:        CodePolicyDenied,
		Reason:      reason,
		Retryable:   false,
		FixableArgs: append([]string(nil), fixableArgs...),
		SafeMessage: "",
		Err:         ErrPolicyDenied,
	}
}

// NewPolicyDeniedErrorFrom preserves a lower-level authorization cause while exposing policy denial.
func NewPolicyDeniedErrorFrom(err error) *ToolError {
	if err == nil {
		return NewPolicyDeniedError("")
	}
	return &ToolError{
		Code:        CodePolicyDenied,
		Reason:      err.Error(),
		Retryable:   false,
		FixableArgs: nil,
		SafeMessage: "",
		Err:         fmt.Errorf("%w: %w", ErrPolicyDenied, err),
	}
}

// NewCapabilityDeniedError reports a tool call outside the active capability view.
func NewCapabilityDeniedError(toolName string, view RegistryViewSnapshot) *ToolError {
	reason := "tool is not available in the active capability view"
	if toolName != "" {
		reason = "tool " + toolName + " is not available in the active capability view"
	}
	if view.ID != "" {
		reason += " " + view.ID
	}
	return &ToolError{
		Code:        CodeCapabilityDenied,
		Reason:      reason,
		Retryable:   false,
		FixableArgs: []string{"tool_name"},
		SafeMessage: "",
		Err:         ErrCapabilityDenied,
	}
}

func decisionError(d Decision) error {
	if d.Allow {
		return nil
	}
	if d.Code == "" {
		d.Code = CodePolicyDenied
	}
	if d.Reason == "" {
		d.Reason = ErrPolicyDenied.Error()
	}
	if d.Err == nil {
		d.Err = ErrPolicyDenied
	}
	return &ToolError{
		Code:        d.Code,
		Reason:      d.Reason,
		Retryable:   d.Retryable,
		FixableArgs: append([]string(nil), d.FixableArgs...),
		SafeMessage: d.SafeMessage,
		Err:         d.Err,
	}
}

func evaluatePolicy(ctx context.Context, p Policy, req PolicyRequest) error {
	if p == nil {
		return nil
	}
	return decisionError(p.Decide(ctx, req))
}

type requirementsPolicyMarker interface {
	enforcesRequirements() bool
}

func policyEnforcesRequirements(p Policy) bool {
	if p == nil {
		return false
	}
	marker, ok := p.(requirementsPolicyMarker)
	return ok && marker.enforcesRequirements()
}

type compositePolicy struct {
	first  Policy
	second Policy
}

func (p compositePolicy) Decide(ctx context.Context, req PolicyRequest) Decision {
	if err := evaluatePolicy(ctx, p.first, req); err != nil {
		if te, ok := AsToolError(err); ok {
			return Decision{
				Allow:       false,
				Code:        te.Code,
				Reason:      te.Reason,
				SafeMessage: te.SafeMessage,
				FixableArgs: te.FixableArgs,
				Retryable:   te.Retryable,
				Err:         te.Err,
			}
		}
		return DenyDecision(err.Error())
	}
	return p.second.Decide(ctx, req)
}

func (p compositePolicy) enforcesRequirements() bool {
	return policyEnforcesRequirements(p.first) || policyEnforcesRequirements(p.second)
}

func composePolicies(first, second Policy) Policy {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return compositePolicy{first: first, second: second}
}
