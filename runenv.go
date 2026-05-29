package toolsy

import "context"

type runEnvContextKey struct{}

// BindEnv attaches a typed application environment to ctx for the duration of a tool call.
func BindEnv[T any](ctx context.Context, env T) context.Context {
	return context.WithValue(ctx, runEnvContextKey{}, env)
}

// EnvFromContext returns the bound application environment, or false when absent or wrong type.
func EnvFromContext[T any](ctx context.Context) (T, bool) {
	var zero T
	raw := ctx.Value(runEnvContextKey{})
	if raw == nil {
		return zero, false
	}
	env, ok := raw.(T)
	return env, ok
}
