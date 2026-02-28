package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTool_Simple(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("add_one", "Add one", func(_ context.Context, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, tool)
	assert.Equal(t, "add_one", tool.Name())
	assert.Equal(t, "Add one", tool.Description())
	params := tool.Parameters()
	require.NotNil(t, params)
}

func TestNewTool_Execute_Success(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("add_one", "Add one", func(_ context.Context, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	res, err := tool.Execute(context.Background(), []byte(`{"x": 5}`))
	require.NoError(t, err)
	var out Result
	require.NoError(t, json.Unmarshal(res, &out))
	assert.Equal(t, 6, out.Y)
}

func TestNewTool_Execute_InvalidJSON(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct{}
	tool, err := NewTool("id", "desc", func(_ context.Context, _ Args) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)
	_, err = tool.Execute(context.Background(), []byte(`{invalid`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestNewTool_Execute_SchemaValidation(t *testing.T) {
	type Args struct {
		Count int `json:"count"`
	}
	type Result struct{}
	tool, err := NewTool("id", "desc", func(_ context.Context, _ Args) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)
	// Wrong type for count (string instead of int) yields schema validation error
	_, err = tool.Execute(context.Background(), []byte(`{"count": "not a number"}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestNewTool_ImplementsTool(t *testing.T) {
	type A struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool[A, R]("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)
	//nolint:staticcheck // interface satisfaction check
	var _ Tool = tool
}

func TestTool_Tags_ReturnsCopy(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	}, WithTags("a", "b"))
	require.NoError(t, err)
	meta, ok := tool.(ToolMetadata)
	require.True(t, ok)
	tags := meta.Tags()
	require.Equal(t, []string{"a", "b"}, tags)
	tags[0] = "mutated"
	tags2 := meta.Tags()
	require.Equal(t, []string{"a", "b"}, tags2)
}

func TestTool_Parameters_ReturnsCopy(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("t", "d", func(_ context.Context, a Args) (R, error) {
		return R{Y: a.X}, nil
	})
	require.NoError(t, err)
	params := tool.Parameters()
	require.NotNil(t, params)
	params["mutated"] = true
	params2 := tool.Parameters()
	_, ok := params2["mutated"]
	require.False(t, ok)
}

// TestTool_Parameters_ShallowCopyNested documents that Parameters() is a shallow copy: nested maps are shared.
func TestTool_Parameters_ShallowCopyNested(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("t", "d", func(_ context.Context, a Args) (R, error) {
		return R{Y: a.X}, nil
	})
	require.NoError(t, err)
	params := tool.Parameters()
	require.NotNil(t, params)
	obj := findSchemaObject(params)
	require.NotNil(t, obj, "expected properties in schema")
	props, ok := obj["properties"].(map[string]any)
	require.True(t, ok)
	// Mutating nested map affects the tool's internal schema (shallow copy).
	props["x"] = "mutated_nested"
	params2 := tool.Parameters()
	obj2 := findSchemaObject(params2)
	require.NotNil(t, obj2)
	props2 := obj2["properties"].(map[string]any)
	assert.Equal(t, "mutated_nested", props2["x"], "nested maps are shared")
}

func BenchmarkExecute(b *testing.B) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct {
		Y int `json:"y"`
	}
	tool, err := NewTool("bench", "desc", func(_ context.Context, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	argsJSON := []byte(`{"x": 42}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tool.Execute(ctx, argsJSON)
	}
}
