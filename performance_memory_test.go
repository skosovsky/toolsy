package toolsy

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func BenchmarkPolicyManifestRich(b *testing.B) {
	input, err := performanceSchema(b, "input")
	if err != nil {
		b.Fatal(err)
	}
	output, err := performanceSchema(b, "output")
	if err != nil {
		b.Fatal(err)
	}
	m := ToolManifest{Name: "groups", Parameters: input, OutputSchema: output}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = cloneManifestForPolicy(m)
	}
}

func TestMemorySnapshotRetainedChurn(t *testing.T) {
	// Arrange: retained callbacks must remain detached across large/small and unique graphs.
	source := map[string]any{"value": []any{"original"}}
	saved := cloneMutableValue(source).(map[string]any)
	// Act.
	for epoch := range 20 {
		graph := map[string]any{}
		for i := range 1500 {
			graph[strconv.Itoa(i)] = map[string]any{"epoch": epoch}
		}
		_ = cloneMutableValue(graph)
		out := cloneMutableValue(source).(map[string]any)
		out["value"].([]any)[0] = epoch
	}
	saved["value"].([]any)[0] = "saved"
	// Assert.
	if source["value"].([]any)[0] != "original" {
		t.Fatal("retained output aliases source")
	}
	next := cloneMutableValue(source).(map[string]any)
	if next["value"].([]any)[0] != "original" {
		t.Fatal("old snapshots poison later clones")
	}
}

func TestMemoryCloneConcurrent(t *testing.T) {
	// Arrange.
	source := map[string]any{}
	source["self"] = source
	source["values"] = []any{source, source}
	var wg sync.WaitGroup
	// Act.
	for worker := range 32 {
		wg.Go(func() {
			for range 100 {
				out := cloneMutableValue(source).(map[string]any)
				out["worker"] = worker
				// Assert: cycle/alias is local to this operation.
				if out["self"].(map[string]any)["worker"] != worker ||
					out["values"].([]any)[1].(map[string]any)["worker"] != worker {
					t.Error("identity lost")
				}
			}
		})
	}
	wg.Wait()
	// Assert.
	if source["worker"] != nil {
		t.Fatal("concurrent source mutation")
	}
}

func TestMemoryCloneNilOverlap(t *testing.T) {
	// Arrange.
	type named []any
	var nilSlice named
	backing := []any{map[string]any{"x": 1}, "second"}
	source := []any{nilSlice, named{}, backing[:1], backing[:2]}
	// Act.
	out := cloneMutableValue(source).([]any)
	out[2].([]any)[0].(map[string]any)["x"] = 2
	// Assert.
	if !reflect.ValueOf(out[0]).IsNil() || reflect.ValueOf(out[1]).IsNil() {
		t.Fatal("nil/empty changed")
	}
	if out[3].([]any)[0].(map[string]any)["x"] != 2 {
		t.Fatal("shared child alias lost")
	}
	out[2].([]any)[0] = "replaced"
	if _, ok := out[3].([]any)[0].(map[string]any); !ok {
		t.Fatal("overlapping slices merged")
	}
	if backing[0].(map[string]any)["x"] != 1 {
		t.Fatal("source changed")
	}
}
