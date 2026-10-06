package toolsy

import (
	"context"
	"fmt"
)

// Authorizer is a narrow error-returning adapter port for host authorization.
// Use NewAuthorizerPolicy and WithPolicy with a stable host-owned policy ID.
type Authorizer interface {
	Authorize(context.Context, PolicyRequest) error
}

type AuthorizerFunc func(context.Context, PolicyRequest) error

func (f AuthorizerFunc) Authorize(ctx context.Context, req PolicyRequest) error {
	if f == nil {
		return NewPolicyDeniedError("authorizer function is nil")
	}
	return f(ctx, req)
}

// NewAuthorizerPolicy captures a required authorizer; nil, typed-nil and nil
// functions fail construction. It never resolves a dependency from RunEnv.
// All authorization failures become nonretryable, noncorrectable policy denials.
// The original cause remains inspectable; structured argument decisions belong
// to Policy/typed argument policy rather than this simple adapter.
func NewAuthorizerPolicy(authorizer Authorizer) (Policy, error) {
	if isNilValue(authorizer) {
		return nil, fmt.Errorf("%w: authorizer is nil", ErrPolicyConfiguration)
	}
	return authorizerPolicy{authorizer: authorizer}, nil
}

type authorizerPolicy struct{ authorizer Authorizer }

func (p authorizerPolicy) Decide(ctx context.Context, req PolicyRequest) Decision {
	if err := p.authorizer.Authorize(ctx, clonePolicyRequest(req)); err != nil {
		decision := DenyDecision(err.Error())
		decision.Err = fmt.Errorf("%w: %w", ErrPolicyDenied, err)
		return decision
	}
	return AllowDecision()
}
