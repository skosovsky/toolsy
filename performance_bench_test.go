package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"os"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// BenchmarkManifestPreDispatch measures the pinned public registry/proxy path
// without native reads, host identity queries, transport, result encoding or
// output validation. It cannot be used as the full performance acceptance.
func BenchmarkManifestPreDispatch(b *testing.B) {
	// Arrange: the exact read schema and output manifest, with a handler that stops
	// before application work. The registry still performs its own safety snapshots.
	input, err := performanceSchema(b, "input")
	if err != nil {
		b.Fatal(err)
	}
	output, err := performanceSchema(b, "output")
	if err != nil {
		b.Fatal(err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		b.Fatal(err)
	}
	handlerCalls := 0
	tool, err := NewProxyTool(
		"zalo_list_groups",
		"diagnostic allocation boundary",
		raw,
		func(context.Context, *RunEnv, []byte, func(Chunk) error) error {
			handlerCalls++
			return NewValidationError("diagnostic stop before native dispatch")
		},
		WithOutputSchema(output),
		WithReadOnly(),
		WithIdempotent(),
	)
	if err != nil {
		b.Fatal(err)
	}
	builder := NewRegistryBuilder(
		WithPolicy(
			"diagnostic-allow-v1",
			PolicyFunc(func(context.Context, PolicyRequest) Decision { return AllowDecision() }),
		),
	)
	builder.Add(tool)
	registry, err := builder.Build()
	if err != nil {
		b.Fatal(err)
	}
	view, err := registry.View(
		RegistryViewSpec{ToolNames: []string{"zalo_list_groups"}, RequiredToolNames: []string{"zalo_list_groups"}},
	)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	args := []byte(`{}`)
	b.ReportAllocs()
	b.ResetTimer()
	// Act.
	for range b.N {
		err = view.Execute(
			ctx,
			ToolCall{ToolName: "zalo_list_groups", Input: ToolInput{ArgsJSON: args}, Env: NewRunEnv(nil)},
			func(Chunk) error { return nil },
		)
		if err == nil {
			b.Fatal("diagnostic stop was lost")
		}
	}
	b.StopTimer()
	// Assert: every call entered the real validated proxy exactly once.
	if handlerCalls != b.N {
		b.Fatal("proxy dispatch skipped or duplicated")
	}
}

func performanceSchema(b *testing.B, kind string) (map[string]any, error) {
	b.Helper()
	raw, err := os.ReadFile("testdata/performance/zalo_list_groups." + kind + ".json")
	if err != nil {
		return nil, err
	}
	value, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	return value.(map[string]any), nil
}

func BenchmarkPerformanceClone(b *testing.B) {
	rich, err := performanceSchema(b, "output")
	if err != nil {
		b.Fatal(err)
	}
	type named map[string]any
	type record struct {
		Values map[string]any
		Items  []any
	}
	shared := map[string]any{"x": []any{1, "two"}}
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, fixture := range []struct {
		name  string
		value any
	}{
		{"tiny", map[string]any{"a": 1}}, {"rich", rich},
		{"named", named{"data": rich}}, {"byot", record{shared, []any{shared}}},
		{"alias", []any{shared, shared}}, {"cycle", cycle},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = cloneMutableValue(fixture.value)
			}
		})
	}
}

func BenchmarkPerformanceDecode(b *testing.B) {
	large, err := os.ReadFile("testdata/performance/groups.payload.json")
	if err != nil {
		b.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		raw     []byte
		invalid bool
	}{
		{"tiny", []byte(`{}`), false}, {"large", large, false}, {"malformed", []byte(`{"x":1,"\u0078":2}`), true},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, err := jsonschemax.Decode(fixture.raw)
				if (err != nil) != fixture.invalid {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPerformanceFreshSchema(b *testing.B) {
	schema, err := performanceSchema(b, "output")
	if err != nil {
		b.Fatal(err)
	}
	validator, err := jsonschemax.Compile(schema)
	if err != nil {
		b.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/performance/groups.payload.json")
	if err != nil {
		b.Fatal(err)
	}
	chunk := Chunk{Event: EventResult, Data: raw, MimeType: MimeTypeJSON}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := prepareResultChunk(chunk, validator); err != nil {
			b.Fatal(err)
		}
	}
}
