package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubAuthorizer struct {
	err error
}

func (a stubAuthorizer) Authorize(_ context.Context, _ ToolManifest, _ ToolInput) error {
	return a.err
}

func TestWithAuthorizer_DenyBeforeExecute(t *testing.T) {
	tool := newMiddlewareMinTool(
		"secret",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			t.Fatal("tool should not run when denied")
			return nil
		},
	)
	denyErr := errors.New("denied")
	reg, err := NewRegistryBuilder().
		Add(tool).
		WithOptions(WithAuthorizer(stubAuthorizer{err: denyErr})).
		Build()
	require.NoError(t, err)

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.ErrorIs(t, err, denyErr)
}

func TestWithAuthorizationMiddleware_Deny(t *testing.T) {
	tool := newMiddlewareMinTool(
		"secret",
		func(_ context.Context, _ RunContext, _ ToolInput, _ func(Chunk) error) error {
			t.Fatal("tool should not run when denied")
			return nil
		},
	)
	denyErr := errors.New("denied")
	reg, err := NewRegistryBuilder().
		Use(WithAuthorization(stubAuthorizer{err: denyErr})).
		Add(tool).
		Build()
	require.NoError(t, err)

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.ErrorIs(t, err, denyErr)
}
