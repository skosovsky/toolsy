package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRunPolicy_ForcedNotInAllowed(t *testing.T) {
	err := ValidateRunPolicy(RunPolicy{
		ForcedTool:   "a",
		AllowedTools: []string{"b", "c"},
	})
	require.Error(t, err)
}

func TestValidateRunPolicy_ForcedInAllowed(t *testing.T) {
	require.NoError(t, ValidateRunPolicy(RunPolicy{
		ForcedTool:   "a",
		AllowedTools: []string{"a", "b"},
	}))
}

func TestValidateRunPolicy_ForcedNotInRequired(t *testing.T) {
	err := ValidateRunPolicy(RunPolicy{
		ForcedTool:    "a",
		RequiredTools: []string{"b"},
	})
	require.Error(t, err)
}

func TestValidateRunPolicy_RequiredNotSubsetOfAllowed(t *testing.T) {
	err := ValidateRunPolicy(RunPolicy{
		RequiredTools: []string{"a", "c"},
		AllowedTools:  []string{"a", "b"},
	})
	require.Error(t, err)
}

func TestValidateRunPolicy_DuplicateAllowed(t *testing.T) {
	err := ValidateRunPolicy(RunPolicy{
		AllowedTools: []string{"a", "a"},
	})
	require.Error(t, err)
}

func TestSession_EnforcesRequiredTools(t *testing.T) {
	toolA := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	toolB := newMiddlewareMinTool(
		"b",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistryBuilder().Add(toolA, toolB).Build()
	require.NoError(t, err)

	sess, err := NewSession(reg, WithRunPolicy(RunPolicy{RequiredTools: []string{"a"}}))
	require.NoError(t, err)
	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "b",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeValidationFailed)

	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "a",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}

func TestSession_EnforcesRunPolicyAllowedTools(t *testing.T) {
	toolA := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	toolB := newMiddlewareMinTool(
		"b",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistryBuilder().Add(toolA, toolB).Build()
	require.NoError(t, err)

	sess, err := NewSession(reg, WithRunPolicy(RunPolicy{AllowedTools: []string{"a"}}))
	require.NoError(t, err)
	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "b",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeValidationFailed)

	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "a",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}

func TestSession_EnforcesForcedTool(t *testing.T) {
	tool := newMiddlewareMinTool(
		"only",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	other := newMiddlewareMinTool(
		"other",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistryBuilder().Add(tool, other).Build()
	require.NoError(t, err)

	sess, err := NewSession(reg, WithRunPolicy(RunPolicy{ForcedTool: "only"}))
	require.NoError(t, err)
	err = sess.Execute(context.Background(), ToolCall{
		ToolName: "other",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.Error(t, err)
}

func TestRegistry_DoesNotEnforceRunPolicy(t *testing.T) {
	toolB := newMiddlewareMinTool(
		"b",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistryBuilder().Add(toolB).Build()
	require.NoError(t, err)

	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "b",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
}
