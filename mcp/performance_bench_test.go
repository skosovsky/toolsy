package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

func performanceExportFixture(b testing.TB) (*toolsy.RegistryView, ExportConfig) {
	b.Helper()
	raw, err := os.ReadFile("../testdata/performance/zalo_list_groups.output.json")
	if err != nil {
		b.Fatal(err)
	}
	value, err := jsonschemax.Decode(raw)
	if err != nil {
		b.Fatal(err)
	}
	payload, err := os.ReadFile("../testdata/performance/groups.payload.json")
	if err != nil {
		b.Fatal(err)
	}
	tool, err := toolsy.NewProxyTool(
		"groups",
		"groups",
		[]byte(`{"type":"object"}`),
		func(_ context.Context, _ *toolsy.RunEnv, _ []byte, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, MimeType: toolsy.MimeTypeJSON, Data: payload})
		},
		toolsy.WithOutputSchema(value.(map[string]any)),
	)
	if err != nil {
		b.Fatal(err)
	}
	reg, err := toolsy.NewRegistry(tool)
	if err != nil {
		b.Fatal(err)
	}
	view, err := reg.View(toolsy.RegistryViewSpec{ToolNames: []string{"groups"}})
	if err != nil {
		b.Fatal(err)
	}
	return view, ExportConfig{
		Names:          []string{"groups"},
		Implementation: &sdk.Implementation{Name: "performance", Version: "1"},
		Resolve: func(context.Context, sdk.Request) (toolsy.CallContext, error) {
			return toolsy.NewCallContext(toolsy.NoSubject{}, toolsy.NoScope{}), nil
		},
		MaxResultBytes: 1 << 20,
		MaxChunks:      1024,
	}
}
func BenchmarkPerformanceExport(b *testing.B) {
	for _, custom := range []bool{false, true} {
		name := "default"
		if custom {
			name = "custom"
		}
		b.Run(name, func(b *testing.B) {
			view, config := performanceExportFixture(b)
			if custom {
				config.Project = func(_ context.Context, _ toolsy.ToolManifest, chunks []toolsy.Chunk) (*sdk.CallToolResult, error) {
					return &sdk.CallToolResult{StructuredContent: json.RawMessage(chunks[0].Data)}, nil
				}
			}
			entries, err := selectExportManifests(view, config)
			if err != nil {
				b.Fatal(err)
			}
			req := &sdk.CallToolRequest{
				Params: &sdk.CallToolParamsRaw{Name: "groups", Arguments: json.RawMessage(`{}`)},
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := exportCall(context.Background(), view, config, entries[0], req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
