package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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
	tool, err := NewDynamicTool("dynamic", "A dynamic tool", schema, func(_ context.Context, argsJSON []byte, yield func([]byte) error) error {
		return yield(argsJSON)
	})
	require.NoError(t, err)
	require.NotNil(t, tool)
	assert.Equal(t, "dynamic", tool.Name())
	assert.Equal(t, "A dynamic tool", tool.Description())

	var res []byte
	err = tool.Execute(context.Background(), []byte(`{"x": 42}`), func(chunk []byte) error {
		res = chunk
		return nil
	})
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
	tool, err := NewDynamicTool("weather", "Weather", schema, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	})
	require.NoError(t, err)

	yieldNop := func([]byte) error { return nil }
	// Missing required field
	err = tool.Execute(context.Background(), []byte(`{}`), yieldNop)
	require.Error(t, err)
	assert.True(t, IsClientError(err))

	// Invalid enum
	err = tool.Execute(context.Background(), []byte(`{"unit": "kelvin"}`), yieldNop)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestNewDynamicTool_InvalidSchema(t *testing.T) {
	t.Parallel()
	// Schema that fails to resolve (type must be string or array of strings per JSON Schema)
	invalidSchema := map[string]any{
		"type": 123,
	}
	_, err := NewDynamicTool("bad", "Bad", invalidSchema, func(_ context.Context, _ []byte, _ func([]byte) error) error {
		return nil
	})
	require.Error(t, err)

	// Nil schema
	_, err = NewDynamicTool("nil", "Nil", nil, func(_ context.Context, _ []byte, _ func([]byte) error) error {
		return nil
	})
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
	tool, err := NewDynamicTool("classify", "Classify", schema, func(_ context.Context, _ []byte, _ func([]byte) error) error {
		return clientErr
	})
	require.NoError(t, err)

	err = tool.Execute(context.Background(), []byte(`{"x": 1}`), func([]byte) error { return nil })
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	var ce *ClientError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "bad request", ce.Reason)

	// Non-ClientError becomes SystemError
	tool2, err := NewDynamicTool("sys", "Sys", schema, func(_ context.Context, _ []byte, _ func([]byte) error) error {
		return errors.New("internal failure")
	})
	require.NoError(t, err)
	err = tool2.Execute(context.Background(), []byte(`{"x": 1}`), func([]byte) error { return nil })
	require.Error(t, err)
	assert.True(t, IsSystemError(err))
}

func TestNewDynamicTool_MetadataOptions(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	tool, err := NewDynamicTool("meta", "Meta", schema, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	}, WithTimeout(30*time.Second), WithTags("a", "b"), WithVersion("1.0"), WithDangerous())
	require.NoError(t, err)

	tm, ok := tool.(ToolMetadata)
	require.True(t, ok, "dynamic tool must implement ToolMetadata")
	assert.Equal(t, 30*time.Second, tm.Timeout())
	assert.Equal(t, []string{"a", "b"}, tm.Tags())
	assert.Equal(t, "1.0", tm.Version())
	assert.True(t, tm.IsDangerous())
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
	tool, err := NewDynamicTool("strict_tool", "Strict", schema, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	}, WithStrict())
	require.NoError(t, err)

	params := tool.Parameters()
	obj := findSchemaObject(params)
	require.NotNil(t, obj, "expected object with properties")
	assert.Equal(t, false, obj["additionalProperties"])
	required, ok := obj["required"].([]any)
	require.True(t, ok)
	assert.Len(t, required, 2)
}

func TestNewDynamicTool_DoesNotMutateInputSchemaMap(t *testing.T) {
	t.Parallel()
	// Nested object with its own properties, $id, and id — all must remain unchanged in caller's map.
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
	tool, err := NewDynamicTool("no_mutate", "No mutate", schemaMap, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	}, WithStrict())
	require.NoError(t, err)
	require.NotNil(t, tool)

	// Root: caller's map must not have been modified (strict/additions apply only to our deep copy).
	assert.Nil(t, schemaMap["required"], "caller root must not have required key added")
	assert.Nil(t, schemaMap["additionalProperties"], "caller root must not have additionalProperties added")
	assert.Equal(t, "https://example.com/root", schemaMap["$id"], "caller root $id must be preserved")

	// Nested object: must still have $id/id and must NOT have additionalProperties/required added by strict.
	assert.Equal(t, "https://example.com/nested", nestedObj["$id"], "caller nested $id must be preserved")
	assert.Equal(t, "nested", nestedObj["id"], "caller nested id must be preserved")
	assert.Nil(t, nestedObj["required"], "caller nested must not have required key added")
	assert.Nil(t, nestedObj["additionalProperties"], "caller nested must not have additionalProperties added")
}

func TestNewDynamicTool_PostConstructMutatingCallerDoesNotAffectToolSchema(t *testing.T) {
	t.Parallel()
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
	}
	tool, err := NewDynamicTool("isolated", "Isolated", schemaMap, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	})
	require.NoError(t, err)
	paramsBefore := tool.Parameters()
	propsBefore, ok := paramsBefore["properties"].(map[string]any)
	require.True(t, ok)
	_, hasX := propsBefore["x"]
	require.True(t, hasX, "tool schema must have property x")
	_, hasYBefore := propsBefore["y"]

	// Mutate caller's map after construction (root and nested).
	schemaMap["mutatedRoot"] = true
	if props, ok := schemaMap["properties"].(map[string]any); ok {
		props["y"] = map[string]any{"type": "string"}
	}

	paramsAfter := tool.Parameters()
	assert.Nil(t, paramsAfter["mutatedRoot"], "tool schema must not reflect caller's root mutation")
	propsAfter, ok := paramsAfter["properties"].(map[string]any)
	require.True(t, ok)
	_, hasYAfter := propsAfter["y"]
	assert.False(t, hasYBefore, "sanity: y was not in initial tool schema")
	assert.False(t, hasYAfter, "tool schema must not reflect caller's nested mutation")
}

func TestNewDynamicTool_MultipleChunks(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"n": map[string]any{"type": "integer"},
		},
		"required": []any{"n"},
	}
	tool, err := NewDynamicTool("stream_dyn", "Stream N chunks", schema, func(_ context.Context, argsJSON []byte, yield func([]byte) error) error {
		var args struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return err
		}
		for i := 0; i < args.N; i++ {
			chunk := []byte{byte('0' + i)}
			if err := yield(chunk); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, tool)

	var chunks [][]byte
	err = tool.Execute(context.Background(), []byte(`{"n": 4}`), func(chunk []byte) error {
		chunks = append(chunks, append([]byte(nil), chunk...))
		return nil
	})
	require.NoError(t, err)
	require.Len(t, chunks, 4)
	assert.Equal(t, []byte("0"), chunks[0])
	assert.Equal(t, []byte("1"), chunks[1])
	assert.Equal(t, []byte("2"), chunks[2])
	assert.Equal(t, []byte("3"), chunks[3])
}

func TestNewDynamicTool_ZeroChunks(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	tool, err := NewDynamicTool("nop_stream", "Side-effect only", schema, func(_ context.Context, _ []byte, _ func([]byte) error) error {
		return nil
	})
	require.NoError(t, err)
	var count int
	err = tool.Execute(context.Background(), []byte(`{}`), func([]byte) error {
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNewDynamicTool_YieldError(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"x": map[string]any{"type": "integer"}},
	}
	yieldErr := errors.New("client closed")
	tool, err := NewDynamicTool("abort_dyn", "Abort on yield", schema, func(_ context.Context, _ []byte, yield func([]byte) error) error {
		_ = yield([]byte("first"))
		return yield([]byte("second"))
	})
	require.NoError(t, err)
	var received [][]byte
	err = tool.Execute(context.Background(), []byte(`{"x": 1}`), func(chunk []byte) error {
		received = append(received, append([]byte(nil), chunk...))
		if string(chunk) == "first" {
			return nil
		}
		return yieldErr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStreamAborted)
	require.Len(t, received, 2)
	assert.Equal(t, []byte("first"), received[0])
	assert.Equal(t, []byte("second"), received[1])
}
