package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithStrict(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	type R struct {
		Y int `json:"y"`
	}

	tool, err := NewTool("strict_tool", "desc", func(_ context.Context, _ *RunEnv, a Args) (R, error) {
		return R{Y: a.X}, nil
	}, WithStrict())
	require.NoError(t, err)

	var res R
	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"x":1}`)},
		func(c Chunk) error { return json.Unmarshal(c.Data, &res) },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Y)

	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"x":1,"extra":2}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestWithTags(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	}, WithTags("tag1", "tag2"))
	require.NoError(t, err)

	assert.Equal(t, []string{"tag1", "tag2"}, tool.Manifest().Tags)
}

func TestWithVersion(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	}, WithVersion("1.0.0"))
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", tool.Manifest().Version)
}

func TestWithDangerous(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	}, WithDangerous())
	require.NoError(t, err)

	assert.True(t, tool.Manifest().Dangerous)
}

func TestWithReadOnly(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	}, WithReadOnly())
	require.NoError(t, err)

	assert.True(t, tool.Manifest().ReadOnly)
}

func TestWithRequirements(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ *RunEnv, _ A) (R, error) {
		return R{}, nil
	}, WithRequiresConfirmation(), WithRequirements(ToolRequirements{
		MemoryAccess: MemoryAccessReadWrite,
		NeedsSession: true,
		Permissions:  []Permission{"admin"},
	}))
	require.NoError(t, err)

	assert.True(t, tool.Manifest().RequiresConfirmation)
	req := tool.Manifest().Requirements
	assert.Equal(t, MemoryAccessReadWrite, req.MemoryAccess)
	assert.True(t, req.NeedsSession)
	assert.Equal(t, []Permission{"admin"}, req.Permissions)
}

func TestToolOptions_Combined(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	type R struct {
		Double int `json:"double"`
	}

	tool, err := NewTool("combined", "desc", func(_ context.Context, _ *RunEnv, a A) (R, error) {
		return R{Double: a.N * 2}, nil
	}, WithStrict(), WithVersion("0.1"))
	require.NoError(t, err)

	var res R
	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil),
		ToolInput{ArgsJSON: []byte(`{"n":21}`)},
		func(c Chunk) error { return json.Unmarshal(c.Data, &res) },
	)
	require.NoError(t, err)
	assert.Equal(t, 42, res.Double)
	assert.Equal(t, "0.1", tool.Manifest().Version)
}
