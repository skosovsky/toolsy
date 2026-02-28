package testutil

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/skosovsky/toolsy"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMockTool(t *testing.T) {
	m := &MockTool{
		NameVal:   "test_tool",
		DescVal:   "For tests",
		ParamsVal: map[string]any{"type": "object"},
		ExecuteFn: func(_ context.Context, _ []byte, yield func([]byte) error) error {
			return yield([]byte(`{"done":true}`))
		},
	}
	assert.Equal(t, "test_tool", m.Name())
	assert.Equal(t, "For tests", m.Description())
	assert.Equal(t, map[string]any{"type": "object"}, m.Parameters())
	var out []byte
	err := m.Execute(context.Background(), []byte(`{}`), func(chunk []byte) error {
		out = chunk
		return nil
	})
	require.NoError(t, err)
	var v struct {
		Done bool `json:"done"`
	}
	require.NoError(t, json.Unmarshal(out, &v))
	assert.True(t, v.Done)
}

func TestNewTestRegistry(t *testing.T) {
	m := &MockTool{NameVal: "m", ExecuteFn: func(_ context.Context, _ []byte, yield func([]byte) error) error {
		return yield([]byte(`{}`))
	}}
	reg := NewTestRegistry(m)
	require.NotNil(t, reg)
	all := reg.GetAllTools()
	require.Len(t, all, 1)
	assert.Equal(t, "m", all[0].Name())
	err := reg.Execute(context.Background(), toolsy.ToolCall{ID: "1", ToolName: "m", Args: []byte(`{}`)}, func([]byte) error { return nil })
	require.NoError(t, err)
}
