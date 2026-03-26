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

	tool, err := NewTool("add_one", "Add one", func(_ context.Context, _ RunContext, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	require.NoError(t, err)
	require.NotNil(t, tool)

	m := tool.Manifest()
	assert.Equal(t, "add_one", m.Name)
	assert.Equal(t, "Add one", m.Description)
	require.NotNil(t, m.Parameters)
}

func TestNewTool_Execute_Success(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct {
		Y int `json:"y"`
	}

	tool, err := NewTool("add_one", "Add one", func(_ context.Context, _ RunContext, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	require.NoError(t, err)

	var out Result
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x": 5}`)},
		func(c Chunk) error {
			assert.JSONEq(t, `{"y":6}`, string(c.Data))
			assert.JSONEq(t, MimeTypeJSON, c.MimeType)
			return json.Unmarshal(c.Data, &out)
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 6, out.Y)
}

func TestNewTool_Execute_InvalidJSON(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type Result struct{}

	tool, err := NewTool("id", "desc", func(_ context.Context, _ RunContext, _ Args) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{invalid`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestNewTool_Execute_SchemaValidation(t *testing.T) {
	type Args struct {
		Count int `json:"count"`
	}
	type Result struct{}

	tool, err := NewTool("id", "desc", func(_ context.Context, _ RunContext, _ Args) (Result, error) {
		return Result{}, nil
	})
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"count":"not a number"}`)},
		func(Chunk) error { return nil },
	)
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

	tool, err := NewTool[A, R]("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	})
	require.NoError(t, err)
	var _ = tool
}

func TestTool_ManifestTags_ReturnsCopy(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithTags("a", "b"))
	require.NoError(t, err)

	m1 := tool.Manifest()
	require.Equal(t, []string{"a", "b"}, m1.Tags)
	m1.Tags[0] = "mutated"

	m2 := tool.Manifest()
	require.Equal(t, []string{"a", "b"}, m2.Tags)
}

func TestTool_ManifestParameters_ReturnsCopy(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, a Args) (R, error) {
		return R{Y: a.X}, nil
	})
	require.NoError(t, err)

	m1 := tool.Manifest()
	require.NotNil(t, m1.Parameters)
	m1.Parameters["mutated"] = true

	m2 := tool.Manifest()
	_, ok := m2.Parameters["mutated"]
	require.False(t, ok)
}

func TestTool_ManifestParameters_ShallowCopyNested(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, a Args) (R, error) {
		return R{Y: a.X}, nil
	})
	require.NoError(t, err)

	m1 := tool.Manifest()
	obj := findSchemaObject(m1.Parameters)
	require.NotNil(t, obj, "expected properties in schema")
	props, ok := obj["properties"].(map[string]any)
	require.True(t, ok)
	props["x"] = "mutated_nested"

	m2 := tool.Manifest()
	obj2 := findSchemaObject(m2.Parameters)
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

	tool, err := NewTool("bench", "desc", func(_ context.Context, _ RunContext, a Args) (Result, error) {
		return Result{Y: a.X + 1}, nil
	})
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	input := ToolInput{ArgsJSON: []byte(`{"x": 42}`)}
	yield := func(Chunk) error { return nil }

	b.ResetTimer()
	for range b.N {
		_ = tool.Execute(ctx, RunContext{}, input, yield)
	}
}

func TestNewProxyTool(t *testing.T) {
	rawSchema := []byte(`{"type":"object","properties":{"x":{"type":"integer"}},"required":["x"]}`)
	tool, err := NewProxyTool(
		"proxy_echo",
		"Echo args as result",
		rawSchema,
		func(_ context.Context, _ RunContext, rawArgs []byte, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: rawArgs, MimeType: MimeTypeJSON})
		},
	)
	require.NoError(t, err)
	require.NotNil(t, tool)

	m := tool.Manifest()
	assert.Equal(t, "proxy_echo", m.Name)
	assert.Equal(t, "Echo args as result", m.Description)
	require.NotNil(t, m.Parameters)

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
	require.NotNil(t, res)

	var out map[string]any
	require.NoError(t, json.Unmarshal(res, &out))
	assert.InDelta(t, 42.0, out["x"].(float64), 1e-9)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}
