package toolsyotel

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/skosovsky/toolsy"
)

// BenchmarkTracingMapping measures middleware costs without recording/exporting
// spans; exporter throughput and network costs are outside this comparison.
func BenchmarkTracingMapping(b *testing.B) {
	for _, vendor := range []bool{false, true} {
		for _, capture := range []bool{false, true} {
			b.Run(fmt.Sprintf("vendor=%t/capture=%t", vendor, capture), func(b *testing.B) {
				tool := &stubTool{
					manifest: toolsy.ToolManifest{Name: "bench"},
					execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
						return yield(toolsy.Chunk{Data: []byte("bounded result"), MimeType: toolsy.MimeTypeText})
					},
				}
				wrapped := WithTracing(
					WithTracerProvider(noop.NewTracerProvider()),
					WithContentCapture(capture),
					WithLangfuseCompatibility(vendor),
				)(
					tool,
				)
				run := toolsy.NewRunEnv(nil)
				input := toolsy.ToolInput{CallID: "bench-call", ArgsJSON: []byte(`{"x":1}`)}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if err := wrapped.Execute(
						b.Context(),
						run,
						input,
						func(toolsy.Chunk) error { return nil },
					); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
