package toolsy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAuthorizer struct {
	err error
}

func (a stubAuthorizer) Authorize(_ context.Context, _ AuthorizationRequest) error {
	return a.err
}

func TestWithAuthorizer_DenyBeforeExecute(t *testing.T) {
	// Arrange.
	tool := newMiddlewareMinTool(
		"secret",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
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

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.ErrorIs(t, err, denyErr)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodePolicyDenied, te.Code)
}

func TestWithAuthorizationMiddleware_Deny(t *testing.T) {
	// Arrange.
	tool := newMiddlewareMinTool(
		"secret",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
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

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.ErrorIs(t, err, denyErr)
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodePolicyDenied, te.Code)
}

func TestWithAuthorizationMiddleware_TrustsOnlyRegistryBoundView(t *testing.T) {
	t.Parallel()

	// Arrange.
	var rootViewID string
	var viewViewID string
	var handlerRan bool
	denyErr := errors.New("missing trusted view")
	tool := newMiddlewareMinTool(
		"secret",
		func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
			handlerRan = true
			return nil
		},
	)
	auth := AuthorizerFunc(func(_ context.Context, req AuthorizationRequest) error {
		if req.Input.CallID == "root" {
			rootViewID = req.View.ID
		}
		if req.Input.CallID == "view" {
			viewViewID = req.View.ID
		}
		if req.View.ID == "" {
			return denyErr
		}
		if req.View.ID != req.CallContext.Metadata.ViewID {
			return errors.New("view metadata mismatch")
		}
		return nil
	})
	reg, err := NewRegistryBuilder().
		Use(WithAuthorization(auth)).
		Add(tool).
		Build()
	require.NoError(t, err)
	view, err := reg.View(RegistryViewSpec{ToolNames: []string{"secret"}})
	require.NoError(t, err)

	// Act.
	rootErr := reg.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{CallID: "root", ArgsJSON: []byte(`{}`)},
		CallContext: CallContext{
			Metadata: CallMetadata{ViewID: "spoofed"},
		},
	}, func(Chunk) error { return nil })
	viewErr := view.Execute(context.Background(), ToolCall{
		ToolName: "secret",
		Input:    ToolInput{CallID: "view", ArgsJSON: []byte(`{}`)},
		CallContext: CallContext{
			Metadata: CallMetadata{ViewID: "spoofed"},
		},
	}, func(Chunk) error { return nil })

	// Assert.
	require.ErrorIs(t, rootErr, denyErr)
	require.NoError(t, viewErr)
	assert.Empty(t, rootViewID)
	assert.Equal(t, view.Snapshot().ID, viewViewID)
	assert.True(t, handlerRan)
}
