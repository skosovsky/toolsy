package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
)

func TestPerformanceExportProjectionFrozenConcurrent(t *testing.T) {
	// Arrange: the callback changes its advertised contract and returns a value
	// valid only for that altered schema.
	view, config := performanceExportFixture(t)
	config.Project = func(_ context.Context, m toolsy.ToolManifest, _ []toolsy.Chunk) (*sdk.CallToolResult, error) {
		m.OutputSchema["required"] = []any{}
		m.OutputSchema["properties"].(map[string]any)["groups"].(map[string]any)["type"] = "string"
		m.Parameters["type"] = "string"
		return &sdk.CallToolResult{StructuredContent: map[string]any{}}, nil
	}
	entries, err := selectExportManifests(view, config)
	if err != nil {
		t.Fatal(err)
	}
	// Act: every projection gets its own map across concurrent calls.
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			req := &sdk.CallToolRequest{
				Params: &sdk.CallToolParamsRaw{Name: "groups", Arguments: json.RawMessage(`{}`)},
			}
			if _, err := exportCall(context.Background(), view, config, entries[0], req); err == nil {
				t.Error("changed projected content bypassed frozen schema")
			}
		})
	}
	wg.Wait()
	// Assert: later snapshots retain both contracts and the original validator.
	m := exportManifestSnapshot(entries[0])
	if m.Parameters["type"] != "object" ||
		m.OutputSchema["properties"].(map[string]any)["groups"].(map[string]any)["type"] != "array" {
		t.Fatal("projection poisoned frozen catalog")
	}
	if entries[0].output == nil {
		t.Fatal("registration did not compile output")
	}
}

func TestPerformanceExportControlFinalSchema(t *testing.T) {
	// Arrange: a host control mapper supplies invalid successful structured content.
	_, config := performanceExportFixture(t)
	tool, err := toolsy.NewProxyTool(
		"groups",
		"groups",
		[]byte(`{"type":"object"}`),
		func(context.Context, *toolsy.RunEnv, []byte, func(toolsy.Chunk) error) error { return toolsy.ErrPause },
		toolsy.WithOutputSchema(
			map[string]any{
				"type":       "object",
				"properties": map[string]any{"n": map[string]any{"type": "integer"}},
				"required":   []any{"n"},
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := toolsy.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"groups"}})
	if err != nil {
		t.Fatal(err)
	}
	config.Control = func(context.Context, *sdk.CallToolRequest, error) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{StructuredContent: map[string]any{"n": "invalid"}}, nil
	}
	entries, err := selectExportManifests(view, config)
	if err != nil {
		t.Fatal(err)
	}
	req := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "groups", Arguments: json.RawMessage(`{}`)}}
	// Act.
	result, err := exportCall(context.Background(), view, config, entries[0], req)
	// Assert.
	if err == nil || result != nil {
		t.Fatal("control projection bypassed final schema validation")
	}
}
