package toolsy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

	tool, err := NewTool("strict_tool", "desc", func(_ context.Context, _ RunContext, a Args) (R, error) {
		return R{Y: a.X}, nil
	}, WithStrict())
	require.NoError(t, err)

	var res R
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x":1}`)},
		func(c Chunk) error { return json.Unmarshal(c.Data, &res) },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Y)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"x":1,"extra":2}`)},
		func(Chunk) error { return nil },
	)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestWithTimeout(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithTimeout(time.Second))
	require.NoError(t, err)

	assert.Equal(t, time.Second, tool.Manifest().Timeout)

	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			assert.Equal(t, EventResult, c.Event)
			assert.JSONEq(t, MimeTypeJSON, c.MimeType)
			return nil
		},
	)
	require.NoError(t, err)
}

func TestWithTags(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithTags("tag1", "tag2"))
	require.NoError(t, err)

	assert.Equal(t, []string{"tag1", "tag2"}, tool.Manifest().Tags)
}

func TestWithVersion(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithVersion("1.0.0"))
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", tool.Manifest().Version)
}

func TestWithDangerous(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithDangerous())
	require.NoError(t, err)

	assert.Equal(t, true, tool.Manifest().Metadata["dangerous"])
}

func TestWithReadOnly(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithReadOnly())
	require.NoError(t, err)

	assert.Equal(t, true, tool.Manifest().Metadata["read_only"])
}

func TestWithMetadata(t *testing.T) {
	type A struct{}
	type R struct{}

	tool, err := NewTool("t", "d", func(_ context.Context, _ RunContext, _ A) (R, error) {
		return R{}, nil
	}, WithMetadata(map[string]any{
		"requires_confirmation": true,
		"sensitivity":           "high",
	}))
	require.NoError(t, err)

	meta := tool.Manifest().Metadata
	assert.Equal(t, true, meta["requires_confirmation"])
	assert.Equal(t, "high", meta["sensitivity"])
}

func TestToolOptions_Combined(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	type R struct {
		Double int `json:"double"`
	}

	tool, err := NewTool("combined", "desc", func(_ context.Context, _ RunContext, a A) (R, error) {
		return R{Double: a.N * 2}, nil
	}, WithStrict(), WithTimeout(time.Millisecond), WithVersion("0.1"))
	require.NoError(t, err)

	var res R
	err = tool.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{"n":21}`)},
		func(c Chunk) error { return json.Unmarshal(c.Data, &res) },
	)
	require.NoError(t, err)
	assert.Equal(t, 42, res.Double)
	assert.Equal(t, "0.1", tool.Manifest().Version)
}
