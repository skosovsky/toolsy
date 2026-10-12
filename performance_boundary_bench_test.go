package toolsy

import (
	"context"
	"os"
	"testing"
)

// BenchmarkMemoryRegistryBoundary isolates the required outer validation boundary.
func BenchmarkMemoryRegistryBoundary(b *testing.B) {
	raw, err := os.ReadFile("testdata/performance/groups.payload.json")
	if err != nil {
		b.Fatal(err)
	}
	chunk := Chunk{Event: EventResult, Data: raw, MimeType: MimeTypeJSON}
	registry, err := NewRegistry()
	if err != nil {
		b.Fatal(err)
	}
	summary := ExecutionSummary{}
	yield := registry.wrapYieldWithCallMeta(
		context.Background(),
		ToolCall{ToolName: "groups"},
		&summary,
		func(Chunk) error { return nil },
	)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err = yield(chunk); err != nil {
			b.Fatal(err)
		}
	}
}
