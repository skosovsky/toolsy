package toolsy_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/testutil"
)

func TestOverrideTool_ReplacesManifest(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:        "sql_run",
			Description: "Run SQL",
			Parameters:  map[string]any{"type": "object"},
		},
	}
	wrapped := toolsy.OverrideTool(base,
		toolsy.WithNewName("dba_query"),
		toolsy.WithNewDescription("Execute complex JOINs. Only use if strictly necessary."),
		toolsy.WithNewParameters(map[string]any{"type": "object", "required": []any{"query"}}),
	)
	m := wrapped.Manifest()
	require.Equal(t, "dba_query", m.Name)
	require.Equal(t, "Execute complex JOINs. Only use if strictly necessary.", m.Description)
	require.Equal(t, []any{"query"}, m.Parameters["required"])
}

func TestOverrideTool_PartialOverride(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:        "original",
			Description: "Original description",
			Parameters:  map[string]any{"x": 1},
		},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("renamed_only"))
	m := wrapped.Manifest()
	require.Equal(t, "renamed_only", m.Name)
	require.Equal(t, "Original description", m.Description)
	require.Equal(t, map[string]any{"x": 1}, m.Parameters)
}

func TestOverrideTool_ExecutesBase(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:       "echo",
			Parameters: map[string]any{"type": "object"},
		},
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, input toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: input.ArgsJSON, MimeType: toolsy.MimeTypeJSON})
		},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("wrapped_echo"))
	var got []byte
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"a":1}`)},
		func(c toolsy.Chunk) error {
			got = c.Data
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []byte(`{"a":1}`), got)
}

func TestOverrideTool_ThreadSafety(_ *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:       "base",
			Parameters: map[string]any{"type": "object"},
		},
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult})
		},
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			w1 := toolsy.OverrideTool(base, toolsy.WithNewName("w1"))
			w2 := toolsy.OverrideTool(base, toolsy.WithNewDescription("desc2"))
			_ = w1.Manifest().Name
			_ = w2.Manifest().Description
			_ = w1.Execute(
				context.Background(),
				toolsy.RunContext{},
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(toolsy.Chunk) error { return nil },
			)
			_ = w2.Execute(
				context.Background(),
				toolsy.RunContext{},
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(toolsy.Chunk) error { return nil },
			)
		})
	}
	wg.Wait()
}

func TestOverrideTool_Parameters_ReturnsCopy(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:       "schema_tool",
			Parameters: map[string]any{"type": "object"},
		},
	}
	overrideSchema := map[string]any{"type": "object", "required": []any{"id"}}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))

	p1 := wrapped.Manifest().Parameters
	p2 := wrapped.Manifest().Parameters
	require.Equal(t, []any{"id"}, p1["required"])
	require.Equal(t, []any{"id"}, p2["required"])

	p1["extra"] = "mutated"
	p3 := wrapped.Manifest().Parameters
	require.NotContains(t, p3, "extra")
	require.Equal(t, []any{"id"}, p3["required"])
}

func TestOverrideTool_Parameters_DeepCopy(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{
			Name:       "base",
			Parameters: map[string]any{"type": "object"},
		},
	}
	nestedProps := map[string]any{"name": map[string]any{"type": "string"}}
	overrideSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"user": map[string]any{"type": "object", "properties": nestedProps}},
		"required":   []any{"user"},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))

	first := wrapped.Manifest().Parameters
	user, ok := first["properties"].(map[string]any)["user"].(map[string]any)
	require.True(t, ok)
	innerProps, ok := user["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "string"}, innerProps["name"])

	nestedProps["name"] = map[string]any{"type": "number"}
	overrideSchema["required"] = []any{"user", "other"}

	second := wrapped.Manifest().Parameters
	require.Equal(t, []any{"user"}, second["required"])
	user2, ok := second["properties"].(map[string]any)["user"].(map[string]any)
	require.True(t, ok)
	innerProps2, ok := user2["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "string"}, innerProps2["name"])
}

func TestOverrideTool_Execute_ChunkToolNameUsesAlias(t *testing.T) {
	base := &testutil.MockTool{
		ManifestVal: toolsy.ToolManifest{Name: "internal_tool", Parameters: map[string]any{"type": "object"}},
		ExecuteFn: func(_ context.Context, _ toolsy.RunContext, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{
				Event:    toolsy.EventResult,
				ToolName: "internal_tool",
				Data:     []byte(`"ok"`),
				MimeType: toolsy.MimeTypeJSON,
			})
		},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("public_alias"))

	var gotChunk toolsy.Chunk
	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error {
			gotChunk = c
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, "public_alias", gotChunk.ToolName)
	require.Equal(t, []byte(`"ok"`), gotChunk.Data)
}
