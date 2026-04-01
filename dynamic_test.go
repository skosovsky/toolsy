package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDynamicTool_Success(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		"required": []any{"x"},
	}
	tool, err := NewDynamicTool(
		"dynamic",
		"A dynamic tool",
		schema,
		func(_ context.Context, _ RunContext, argsJSON []byte, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: argsJSON, MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)
	require.NotNil(t, tool)
	assert.Equal(t, "dynamic", tool.Manifest().Name)
	assert.Equal(t, "A dynamic tool", tool.Manifest().Description)

	var res []byte
	err = tool.Execute(
		context.Background(),
		RunContext{},
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

func TestNewDynamicTool_ValidationError(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"unit": map[string]any{"type": "string", "enum": []any{"celsius", "fahrenheit"}},
		},
		"required": []any{"unit"},
	}
	tool, err := NewDynamicTool(
		"weather",
		"Weather",
		schema,
		func(_ context.Context, _ RunContext, _ []byte, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)

	yieldNop := func(Chunk) error { return nil }
	err = tool.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, yieldNop)
	require.Error(t, err)
	assert.True(t, IsClientError(err))

	err = tool.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{"unit": "kelvin"}`)}, yieldNop)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestNewDynamicTool_InvalidSchema(t *testing.T) {
	t.Parallel()
	invalidSchema := map[string]any{"type": 123}
	_, err := NewDynamicTool(
		"bad",
		"Bad",
		invalidSchema,
		func(_ context.Context, _ RunContext, _ []byte, _ func(Chunk) error) error {
			return nil
		},
	)
	require.Error(t, err)

	_, err = NewDynamicTool(
		"nil",
		"Nil",
		nil,
		func(_ context.Context, _ RunContext, _ []byte, _ func(Chunk) error) error {
			return nil
		},
	)
	require.Error(t, err)
}

func TestNewDynamicTool_NilHandler(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	_, err := NewDynamicTool("no_handler", "No handler", schema, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dynamic tool handler must not be nil")
}

func TestNewDynamicTool_ErrorClassification(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "integer"}},
	}
	clientErr := &ClientError{Reason: "bad request"}
	tool, err := NewDynamicTool(
		"classify",
		"Classify",
		schema,
		func(_ context.Context, _ RunContext, _ []byte, _ func(Chunk) error) error {
			return clientErr
		},
	)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	var ce *ClientError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "bad request", ce.Reason)

	tool2, err := NewDynamicTool(
		"sys",
		"Sys",
		schema,
		func(_ context.Context, _ RunContext, _ []byte, _ func(Chunk) error) error {
			return errors.New("internal failure")
		},
	)
	require.NoError(t, err)
	err = tool2.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x": 1}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.True(t, IsSystemError(err))
}

func TestNewDynamicTool_MetadataOptions(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	tool, err := NewDynamicTool(
		"meta",
		"Meta",
		schema,
		func(_ context.Context, _ RunContext, _ []byte, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{}`), MimeType: MimeTypeJSON})
		},
		WithTags("a", "b"),
		WithVersion("1.0"),
		WithDangerous(),
	)
	require.NoError(t, err)

	m := tool.Manifest()
	assert.Equal(t, []string{"a", "b"}, m.Tags)
	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, true, m.Metadata["dangerous"])
}

func TestNewDynamicTool_StrictOption(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "string"},
			"b": map[string]any{"type": "integer"},
		},
	}
	tool, err := NewDynamicTool(
		"strict_tool",
		"Strict",
		schema,
		func(_ context.Context, _ RunContext, _ []byte, yield func(Chunk) error) error {
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

func TestNewDynamicTool_DoesNotMutateInputSchemaMap(t *testing.T) {
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
	tool, err := NewDynamicTool(
		"no_mutate",
		"No mutate",
		schemaMap,
		func(_ context.Context, _ RunContext, _ []byte, yield func(Chunk) error) error {
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

func TestNewDynamicTool_PostConstructMutatingCallerDoesNotAffectToolSchema(t *testing.T) {
	t.Parallel()
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
	}
	tool, err := NewDynamicTool(
		"isolated",
		"Isolated",
		schemaMap,
		func(_ context.Context, _ RunContext, _ []byte, yield func(Chunk) error) error {
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
