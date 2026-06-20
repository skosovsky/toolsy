package toolsy

import "context"

// Authorizer performs runtime authorization before tool execution.
type Authorizer interface {
	Authorize(ctx context.Context, req AuthorizationRequest) error
}

// AuthorizerFunc adapts a function to Authorizer.
type AuthorizerFunc func(context.Context, AuthorizationRequest) error

// Authorize implements Authorizer.
func (f AuthorizerFunc) Authorize(ctx context.Context, req AuthorizationRequest) error {
	if f == nil {
		return NewPolicyDeniedError("authorizer function is nil")
	}
	return f(ctx, req)
}

// WithAuthorizer configures registry-level authorization executed before tools run.
func WithAuthorizer(a Authorizer) RegistryOption {
	return func(o *registryOptions) {
		o.authorizer = a
	}
}

// WithAuthorization returns middleware that delegates to Authorizer on the bound RunEnv or registry option.
func WithAuthorization(auth Authorizer) Middleware {
	return func(next Tool) Tool {
		return &authorizationTool{
			toolBase: toolBase{next: next},
			auth:     auth,
		}
	}
}

type authorizationTool struct {
	toolBase

	auth Authorizer
}

func (t *authorizationTool) Execute(
	ctx context.Context,
	run *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	if t.auth == nil {
		return t.next.Execute(ctx, run, input, yield)
	}
	req := AuthorizationRequest{
		Manifest:    cloneManifestForPolicy(t.next.Manifest()),
		Input:       input.Clone(),
		CallContext: run.CallContext(),
		View:        run.RegistryViewSnapshot(),
	}
	if err := t.auth.Authorize(ctx, req); err != nil {
		if _, ok := AsToolError(err); ok {
			return err
		}
		return NewPolicyDeniedErrorFrom(err)
	}
	return t.next.Execute(ctx, run, input, yield)
}
