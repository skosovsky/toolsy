package toolsy_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/testutil"
)

func TestOverrideTool_ReplacesMetadata(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "sql_run",
		DescVal:   "Run SQL",
		ParamsVal: map[string]any{"type": "object"},
	}
	wrapped := toolsy.OverrideTool(base,
		toolsy.WithNewName("dba_query"),
		toolsy.WithNewDescription("Execute complex JOINs. Only use if strictly necessary."),
		toolsy.WithNewParameters(map[string]any{"type": "object", "required": []any{"query"}}),
	)
	require.Equal(t, "dba_query", wrapped.Name())
	require.Equal(t, "Execute complex JOINs. Only use if strictly necessary.", wrapped.Description())
	require.Equal(t, []any{"query"}, wrapped.Parameters()["required"])
}

func TestOverrideTool_PartialOverride(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "original",
		DescVal:   "Original description",
		ParamsVal: map[string]any{"x": 1},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("renamed_only"))
	require.Equal(t, "renamed_only", wrapped.Name())
	require.Equal(t, "Original description", wrapped.Description())
	require.Equal(t, map[string]any{"x": 1}, wrapped.Parameters())
}

func TestOverrideTool_ExecutesBase(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "echo",
		ExecuteFn: func(_ context.Context, args []byte, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args})
		},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("wrapped_echo"))
	var got []byte
	err := wrapped.Execute(context.Background(), []byte(`{"a":1}`), func(c toolsy.Chunk) error {
		got = c.Data
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []byte(`{"a":1}`), got)
}

func TestOverrideTool_ThreadSafety(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "base",
		ExecuteFn: func(_ context.Context, _ []byte, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult})
		},
	}
	require.NotNil(t, base)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w1 := toolsy.OverrideTool(base, toolsy.WithNewName("w1"))
			w2 := toolsy.OverrideTool(base, toolsy.WithNewDescription("desc2"))
			_ = w1.Name()
			_ = w2.Description()
			_ = w1.Execute(context.Background(), []byte(`{}`), func(toolsy.Chunk) error { return nil })
			_ = w2.Execute(context.Background(), []byte(`{}`), func(toolsy.Chunk) error { return nil })
		}()
	}
	wg.Wait()
}

func TestOverrideTool_Parameters_ReturnsShallowCopy(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "schema_tool",
		ParamsVal: map[string]any{"type": "object"},
	}
	overrideSchema := map[string]any{"type": "object", "required": []any{"id"}}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))
	p1 := wrapped.Parameters()
	p2 := wrapped.Parameters()
	require.Equal(t, []any{"id"}, p1["required"])
	require.Equal(t, []any{"id"}, p2["required"])
	// Mutating the returned map must not affect the wrapper's internal state or the next call's result.
	p1["extra"] = "mutated"
	p3 := wrapped.Parameters()
	require.NotContains(t, p3, "extra", "Parameters() must return a new map each time; mutation must not leak")
	require.Equal(t, []any{"id"}, p3["required"])
}

func TestOverrideTool_Parameters_NestedMutationDoesNotAffectStoredSchema(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "base",
		ParamsVal: map[string]any{"type": "object"},
	}
	// Nested schema typical for JSON Schema: properties -> object -> properties
	nestedProps := map[string]any{"name": map[string]any{"type": "string"}}
	overrideSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"user": map[string]any{"type": "object", "properties": nestedProps}},
		"required":   []any{"user"},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))
	first := wrapped.Parameters()
	require.Equal(t, "object", first["type"])
	user, ok := first["properties"].(map[string]any)["user"].(map[string]any)
	require.True(t, ok)
	innerProps, ok := user["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "string"}, innerProps["name"])

	// Mutate the original nested structure after registration
	nestedProps["name"] = map[string]any{"type": "number"}
	overrideSchema["required"] = []any{"user", "other"}

	// Wrapper must still return the original schema (deep copy was taken at WithNewParameters)
	second := wrapped.Parameters()
	require.Equal(t, []any{"user"}, second["required"], "required must be unchanged by caller mutation")
	user2, ok := second["properties"].(map[string]any)["user"].(map[string]any)
	require.True(t, ok)
	innerProps2, ok := user2["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "string"}, innerProps2["name"], "nested properties must be unchanged by caller mutation")
}

// TestOverrideTool_Parameters_StringSliceAndStringMapCopied verifies that []string and map[string]string
// in the override schema are defensively copied so caller mutation does not affect the stored schema.
func TestOverrideTool_Parameters_StringSliceAndStringMapCopied(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "base",
		ParamsVal: map[string]any{"type": "object"},
	}
	requiredSlice := []string{"a", "b"}
	extraMap := map[string]string{"format": "date", "pattern": "^x"}
	overrideSchema := map[string]any{
		"type":     "object",
		"required": requiredSlice,
		"x-extra":  extraMap,
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))
	p1 := wrapped.Parameters()
	require.Equal(t, []string{"a", "b"}, p1["required"])
	require.Equal(t, map[string]string{"format": "date", "pattern": "^x"}, p1["x-extra"])

	// Mutate the original slices/maps after registration
	requiredSlice[0] = "mutated"
	requiredSlice = append(requiredSlice, "c")
	extraMap["format"] = "date-time"
	delete(extraMap, "pattern")

	p2 := wrapped.Parameters()
	require.Equal(t, []string{"a", "b"}, p2["required"], "[]string must be copied at WithNewParameters")
	require.Equal(t, map[string]string{"format": "date", "pattern": "^x"}, p2["x-extra"], "map[string]string must be copied at WithNewParameters")
}

// TestOverrideTool_Execute_ChunkToolNameUsesAlias verifies that when the base tool sets Chunk.ToolName,
// OverrideTool normalizes it to the override name so runtime output is consistent with the wrapper's Name().
func TestOverrideTool_Execute_ChunkToolNameUsesAlias(t *testing.T) {
	base := &testutil.MockTool{
		NameVal: "internal_tool",
		ExecuteFn: func(_ context.Context, _ []byte, yield func(toolsy.Chunk) error) error {
			// Base tool sets ToolName to its own name; wrapper should override to alias.
			return yield(toolsy.Chunk{Event: toolsy.EventResult, ToolName: "internal_tool", RawData: "ok"})
		},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("public_alias"))
	var gotChunk toolsy.Chunk
	err := wrapped.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		gotChunk = c
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "public_alias", gotChunk.ToolName, "Chunk.ToolName must be the override name, not the base tool's")
	require.Equal(t, "ok", gotChunk.RawData)
}

// metadataMock implements Tool and ToolMetadata for testing OverrideTool delegation.
type metadataMock struct {
	*testutil.MockTool
	timeout    time.Duration
	tags       []string
	version    string
	isDangerous bool
}

func (m *metadataMock) Timeout() time.Duration     { return m.timeout }
func (m *metadataMock) Tags() []string              { return m.tags }
func (m *metadataMock) Version() string             { return m.version }
func (m *metadataMock) IsDangerous() bool           { return m.isDangerous }
func (m *metadataMock) IsReadOnly() bool            { return false }
func (m *metadataMock) RequiresConfirmation() bool  { return false }
func (m *metadataMock) Sensitivity() string         { return "" }

// TestOverrideTool_ToolMetadataDelegation ensures OverrideTool delegates Timeout/Tags/Version/IsDangerous
// to the base tool via embedded toolBase, so Registry.Execute uses correct metadata for timeout and behavior.
func TestOverrideTool_ToolMetadataDelegation(t *testing.T) {
	base := &metadataMock{
		MockTool:    &testutil.MockTool{NameVal: "base", DescVal: "Base", ParamsVal: map[string]any{"type": "object"}},
		timeout:     33 * time.Second,
		tags:        []string{"alpha", "beta"},
		version:     "1.2.3",
		isDangerous: true,
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewName("alias"), toolsy.WithNewDescription("Overridden desc"))
	require.Equal(t, "alias", wrapped.Name())
	require.Equal(t, "Overridden desc", wrapped.Description())
	require.Equal(t, map[string]any{"type": "object"}, wrapped.Parameters())
	// ToolMetadata must be delegated to base (Registry relies on this for timeout, etc.)
	meta, ok := wrapped.(toolsy.ToolMetadata)
	require.True(t, ok, "OverrideTool must implement ToolMetadata via delegation")
	require.Equal(t, 33*time.Second, meta.Timeout())
	require.Equal(t, []string{"alpha", "beta"}, meta.Tags())
	require.Equal(t, "1.2.3", meta.Version())
	require.True(t, meta.IsDangerous())
}

// TestOverrideTool_Parameters_MapSliceDeepCopy verifies that []map[string]any in the override schema
// is defensively deep-copied so caller mutation does not affect the stored schema.
func TestOverrideTool_Parameters_MapSliceDeepCopy(t *testing.T) {
	base := &testutil.MockTool{
		NameVal:   "base",
		ParamsVal: map[string]any{"type": "object"},
	}
	item := map[string]any{"key": "value", "nested": map[string]any{"a": 1}}
	overrideSchema := map[string]any{
		"type": "object",
		"items": []map[string]any{item, {"second": true}},
	}
	wrapped := toolsy.OverrideTool(base, toolsy.WithNewParameters(overrideSchema))
	p1 := wrapped.Parameters()
	items, ok := p1["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	require.Equal(t, "value", items[0]["key"])
	require.Equal(t, map[string]any{"a": 1}, items[0]["nested"])
	// Mutate the original slice and nested maps after registration
	item["key"] = "mutated"
	item["nested"].(map[string]any)["a"] = 999
	overrideSchema["items"] = append(overrideSchema["items"].([]map[string]any), map[string]any{"third": true})
	p2 := wrapped.Parameters()
	items2, ok := p2["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items2, 2, "stored schema must not gain extra elements from caller mutation")
	require.Equal(t, "value", items2[0]["key"], "[]map[string]any entries must be deep-copied at WithNewParameters")
	require.Equal(t, map[string]any{"a": 1}, items2[0]["nested"], "nested map inside []map[string]any must be copied")
}
