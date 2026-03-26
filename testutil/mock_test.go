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
		ManifestVal: toolsy.ToolManifest{
			Name:        "test_tool",
			Description: "For tests",
			Parameters:  map[string]any{"type": "object"},
		},
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(
				toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`{"done":true}`), MimeType: toolsy.MimeTypeJSON},
			)
		},
	}

	manifest := m.Manifest()
	assert.Equal(t, "test_tool", manifest.Name)
	assert.Equal(t, "For tests", manifest.Description)
	assert.Equal(t, map[string]any{"type": "object"}, manifest.Parameters)

	var out []byte
	err := m.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error {
			out = c.Data
			return nil
		},
	)
	require.NoError(t, err)

	var v struct {
		Done bool `json:"done"`
	}
	require.NoError(t, json.Unmarshal(out, &v))
	assert.True(t, v.Done)
}

func TestNewTestRegistry(t *testing.T) {
	m := &MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "m", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(
			_ context.Context,
			_ toolsy.RunContext,
			_ toolsy.ToolInput,
			yield func(toolsy.Chunk) error,
		) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`{}`), MimeType: toolsy.MimeTypeJSON})
		},
	}
	reg := NewTestRegistry(m)
	require.NotNil(t, reg)

	all := reg.GetAllTools()
	require.Len(t, all, 1)
	assert.Equal(t, "m", all[0].Manifest().Name)

	err := reg.Execute(
		context.Background(),
		toolsy.ToolCall{ID: "1", ToolName: "m", Input: toolsy.ToolInput{ArgsJSON: []byte(`{}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
}
