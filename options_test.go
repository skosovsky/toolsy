package toolsy

import (
	"context"
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
	tool, err := NewTool("strict_tool", "desc", func(_ context.Context, a Args) (R, error) {
		return R{Y: a.X}, nil
	}, WithStrict())
	require.NoError(t, err)
	require.NotNil(t, tool)
	// Valid args
	var res []byte
	err = tool.Execute(context.Background(), []byte(`{"x":1}`), func(chunk []byte) error {
		res = chunk
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	// Extra property should fail schema validation (strict mode)
	err = tool.Execute(context.Background(), []byte(`{"x":1,"extra":2}`), func([]byte) error { return nil })
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestWithTimeout(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	}, WithTimeout(time.Second))
	require.NoError(t, err)
	require.NotNil(t, tool)
	if meta, ok := tool.(ToolMetadata); ok {
		assert.Equal(t, time.Second, meta.Timeout())
	}
	var res []byte
	err = tool.Execute(context.Background(), []byte(`{}`), func(chunk []byte) error {
		res = chunk
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestWithTags(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	}, WithTags("tag1", "tag2"))
	require.NoError(t, err)
	require.NotNil(t, tool)
	if meta, ok := tool.(ToolMetadata); ok {
		assert.Equal(t, []string{"tag1", "tag2"}, meta.Tags())
	}
}

func TestWithVersion(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	}, WithVersion("1.0.0"))
	require.NoError(t, err)
	require.NotNil(t, tool)
	if meta, ok := tool.(ToolMetadata); ok {
		assert.Equal(t, "1.0.0", meta.Version())
	}
}

func TestWithDangerous(t *testing.T) {
	type A struct{}
	type R struct{}
	tool, err := NewTool("t", "d", func(_ context.Context, _ A) (R, error) {
		return R{}, nil
	}, WithDangerous())
	require.NoError(t, err)
	require.NotNil(t, tool)
	if meta, ok := tool.(ToolMetadata); ok {
		assert.True(t, meta.IsDangerous())
	}
}

func TestToolOptions_Combined(t *testing.T) {
	type A struct {
		N int `json:"n"`
	}
	type R struct {
		Double int `json:"double"`
	}
	tool, err := NewTool("combined", "desc", func(_ context.Context, a A) (R, error) {
		return R{Double: a.N * 2}, nil
	}, WithStrict(), WithTimeout(time.Millisecond), WithVersion("0.1"))
	require.NoError(t, err)
	require.NotNil(t, tool)
	var res []byte
	err = tool.Execute(context.Background(), []byte(`{"n":21}`), func(chunk []byte) error {
		res = chunk
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, res)
}
