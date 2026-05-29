package toolsy

import "context"

// Authorizer performs runtime authorization before tool execution.
type Authorizer interface {
	Authorize(ctx context.Context, manifest ToolManifest, input ToolInput) error
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
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) error {
	if t.auth == nil {
		return t.next.Execute(ctx, run, input, yield)
	}
	if err := t.auth.Authorize(ctx, t.next.Manifest(), input); err != nil {
		return err
	}
	return t.next.Execute(ctx, run, input, yield)
}
