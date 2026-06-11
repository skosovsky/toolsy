package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDynamicTool(
	name, description string,
	schema map[string]any,
	fn func(context.Context, *RunEnv, map[string]any, func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	return NewDynamicToolFromSpec(DynamicToolSpec{
		Name:        name,
		Description: description,
		Schema:      MapSchemaProvider(schema),
		Handler:     fn,
		Options:     opts,
	})
}

func TestNewDynamicToolFromSpec_Success(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		"required": []any{"x"},
	}
	tool, err := newDynamicTool(
		"dynamic",
		"A dynamic tool",
		schema,
		func(_ context.Context, _ *RunEnv, decoded map[string]any, yield func(Chunk) error) error {
			data, err := json.Marshal(decoded)
			if err != nil {
				return err
			}
			return yield(Chunk{Event: EventResult, Data: data, MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)
	require.NotNil(t, tool)
	assert.Equal(t, "dynamic", tool.Manifest().Name)
	assert.Equal(t, "A dynamic tool", tool.Manifest().Description)

	var res []byte
	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"x": 42}`)},
		func(c Chunk) error {
			res = c.Data
			return nil
		},
	)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(res, &out))
	assert.InDelta(t, 42.0, out["x"].(float64), 1e-9)
}

func TestNewDynamicToolFromSpec_ValidationError(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"unit": map[string]any{"type": "string", "enum": []any{"celsius", "fahrenheit"}},
		},
		"required": []any{"unit"},
	}
	tool, err := newDynamicTool(
		"weather",
		"Weather",
		schema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)

	yieldNop := func(Chunk) error { return nil }
	err = tool.Execute(context.Background(), NewRunEnv(nil), ToolInput{ArgsJSON: []byte(`{}`)}, yieldNop)
	require.Error(t, err)
	requireClientCorrectable(t, err)

	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"unit": "kelvin"}`)},
		yieldNop,
	)
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestNewDynamicToolFromSpec_InvalidSchema(t *testing.T) {
	t.Parallel()
	invalidSchema := map[string]any{"type": 123}
	_, err := newDynamicTool(
		"bad",
		"Bad",
		invalidSchema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, _ func(Chunk) error) error {
			return nil
		},
	)
	require.Error(t, err)

	_, err = NewDynamicToolFromSpec(DynamicToolSpec{
		Name:        "nil",
		Description: "Nil",
		Schema:      nil,
		Handler:     func(_ context.Context, _ *RunEnv, _ map[string]any, _ func(Chunk) error) error { return nil },
	})
	require.Error(t, err)
}

func TestNewDynamicToolFromSpec_NilHandler(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	_, err := NewDynamicToolFromSpec(DynamicToolSpec{
		Name:        "no_handler",
		Description: "No handler",
		Schema:      MapSchemaProvider(schema),
		Handler:     nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dynamic tool handler must not be nil")
}

func TestNewDynamicToolFromSpec_ErrorClassification(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "integer"}},
	}
	clientErr := NewValidationError("bad request")
	tool, err := newDynamicTool(
		"classify",
		"Classify",
		schema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, _ func(Chunk) error) error {
			return clientErr
		},
	)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	requireClientCorrectable(t, err)
	var ce *ToolError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "bad request", ce.Reason)

	tool2, err := newDynamicTool(
		"sys",
		"Sys",
		schema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, _ func(Chunk) error) error {
			return errors.New("internal failure")
		},
	)
	require.NoError(t, err)
	err = tool2.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	requireSystemToolError(t, err)
}

func TestNewDynamicToolFromSpec_RequirementsOptions(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	tool, err := newDynamicTool(
		"meta",
		"Meta",
		schema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
		WithTags("a", "b"),
		WithVersion("1.0"),
		WithDangerous(),
		WithRequirements(ToolRequirements{MemoryAccess: MemoryAccessRead}),
	)
	require.NoError(t, err)

	m := tool.Manifest()
	assert.Equal(t, []string{"a", "b"}, m.Tags)
	assert.Equal(t, "1.0", m.Version)
	assert.True(t, m.Dangerous)
	assert.Equal(t, MemoryAccessRead, m.Requirements.MemoryAccess)
}

func TestNewDynamicToolFromSpec_StrictOption(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "string"},
			"b": map[string]any{"type": "integer"},
		},
	}
	tool, err := newDynamicTool(
		"strict_tool",
		"Strict",
		schema,
		func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
		WithStrict(),
	)
	require.NoError(t, err)

	params := tool.Manifest().Parameters
	obj := findSchemaObject(params)
	require.NotNil(t, obj, "expected object with properties")
	assert.Equal(t, false, obj["additionalProperties"])
	required, ok := obj["required"].([]any)
	require.True(t, ok)
	assert.Len(t, required, 2)
}

func TestNewDynamicToolFromSpec_DoesNotMutateInputSchemaMap(t *testing.T) {
	t.Parallel()
	nestedObj := map[string]any{
		"type":       "object",
		"$id":        "https://example.com/nested",
		"id":         "nested",
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
	}
	schemaMap := map[string]any{
		"type": "object",
		"$id":  "https://example.com/root",
		"properties": map[string]any{
			"x":      map[string]any{"type": "integer"},
			"nested": nestedObj,
		},
	}
	tool, err := newDynamicTool(
		"no_mutate",
		"No mutate",
		schemaMap,
		func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
		WithStrict(),
	)
	require.NoError(t, err)
	require.NotNil(t, tool)

	assert.Nil(t, schemaMap["required"])
	assert.Nil(t, schemaMap["additionalProperties"])
	assert.Equal(t, "https://example.com/root", schemaMap["$id"])

	assert.Equal(t, "https://example.com/nested", nestedObj["$id"])
	assert.Equal(t, "nested", nestedObj["id"])
	assert.Nil(t, nestedObj["required"])
	assert.Nil(t, nestedObj["additionalProperties"])
}

func TestNewDynamicToolFromSpec_PostConstructMutatingCallerDoesNotAffectToolSchema(t *testing.T) {
	t.Parallel()
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
	}
	tool, err := newDynamicTool(
		"isolated",
		"Isolated",
		schemaMap,
		func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)

	paramsBefore := tool.Manifest().Parameters
	propsBefore, ok := paramsBefore["properties"].(map[string]any)
	require.True(t, ok)
	_, hasX := propsBefore["x"]
	require.True(t, hasX)

	schemaMap["mutatedRoot"] = true
	if props, okProps := schemaMap["properties"].(map[string]any); okProps {
		props["y"] = map[string]any{"type": "string"}
	}

	paramsAfter := tool.Manifest().Parameters
	propsAfter, ok := paramsAfter["properties"].(map[string]any)
	require.True(t, ok)
	_, hasY := propsAfter["y"]
	require.False(t, hasY)
	_, mutatedRoot := paramsAfter["mutatedRoot"]
	require.False(t, mutatedRoot)
}
