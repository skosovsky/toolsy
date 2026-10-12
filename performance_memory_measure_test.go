package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"strconv"
	"sync"
	"testing"
	"time"
)

type memoryWindow struct {
	Name             string  `json:"Name"`
	Iterations       int     `json:"Iterations"`
	DurationNS       int64   `json:"DurationNS"`
	AllocBytes       uint64  `json:"AllocBytes"`
	NaturalGC        uint32  `json:"NaturalGC"`
	STWNS            uint64  `json:"STWNS"`
	GCCPUSeconds     float64 `json:"GCCPUSeconds"`
	AssistCPUSeconds float64 `json:"AssistCPUSeconds"`
	SampledPeak      uint64  `json:"SampledPeak"`
	Retained         uint64  `json:"Retained"`
}

func memoryCPU() (float64, float64) {
	samples := []metrics.Sample{
		{Name: "/cpu/classes/gc/total:cpu-seconds"},
		{Name: "/cpu/classes/gc/mark/assist:cpu-seconds"},
	}
	metrics.Read(samples)
	return samples[0].Value.Float64(), samples[1].Value.Float64()
}

// TestMemoryMeasurement is opt-in diagnostics, shared unchanged with v0.20.1.
//
//nolint:gocognit // One fixed diagnostic protocol keeps all windows and checkpoints comparable.
func TestMemoryMeasurement(t *testing.T) {
	dir := os.Getenv("TOOLSY_MEMORY_OUT")
	if dir == "" {
		t.Skip("set TOOLSY_MEMORY_OUT for fixed retained/GC diagnostic")
	}
	// Arrange: keep a real compiled registry alive at every retained checkpoint.
	tool := performanceToolForMemory(t)
	registry, err := NewRegistryBuilder(
		WithPolicy("allow", PolicyFunc(func(context.Context, PolicyRequest) Decision { return AllowDecision() })),
	).Add(tool).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	call := ToolCall{ToolName: "groups", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	run := func() {
		if e := registry.Execute(context.Background(), call, func(Chunk) error { return nil }); e != nil {
			t.Error(e)
		}
	}
	for range 100 {
		run()
	}
	runtime.GC()
	var checkpoints []memoryWindow
	var saved []any
	for _, name := range []string{"fixed-1", "fixed-2", "fixed-3", "concurrent-churn", "large-small", "registry-churn", "retained-snapshots"} {
		runtime.GC() // outside natural window
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		cpu, assist := memoryCPU()
		start := time.Now()
		peak := before.HeapAlloc
		calls := 300
		for i := range calls {
			switch name {
			case "concurrent-churn":
				var wg sync.WaitGroup
				for worker := range 8 {
					wg.Go(func() {
						graph := map[string]any{"id": fmt.Sprint(i, worker), "child": []any{map[string]any{"value": i}}}
						_ = cloneMutableValue(graph)
						run()
					})
				}
				wg.Wait()
			case "large-small":
				if i%30 == 0 {
					graph := map[string]any{}
					for j := range 3000 {
						graph[strconv.Itoa(j)] = map[string]any{"x": i}
					}
					_ = cloneMutableValue(graph)
				}
				_ = cloneMutableValue(map[string]any{"small": i})
				run()
			case "registry-churn":
				temp := performanceToolForMemory(t)
				r, e := NewRegistryBuilder().Add(temp).Build()
				if e != nil {
					t.Fatal(e)
				}
				if e = r.Execute(context.Background(), call, func(Chunk) error { return nil }); e != nil {
					t.Fatal(e)
				}
			case "retained-snapshots":
				if i < 30 {
					saved = append(saved, cloneManifestForPolicy(tool.Manifest()))
				}
				run()
			default:
				run()
			}
			var sample runtime.MemStats
			runtime.ReadMemStats(&sample)
			if sample.HeapAlloc > peak {
				peak = sample.HeapAlloc
			}
		}
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		cpuAfter, assistAfter := memoryCPU()
		runtime.GC()
		var retained runtime.MemStats
		runtime.ReadMemStats(&retained)
		checkpoints = append(
			checkpoints,
			memoryWindow{
				name,
				calls,
				elapsed.Nanoseconds(),
				after.TotalAlloc - before.TotalAlloc,
				after.NumGC - before.NumGC,
				after.PauseTotalNs - before.PauseTotalNs,
				cpuAfter - cpu,
				assistAfter - assist,
				peak,
				retained.HeapAlloc,
			},
		)
		if name == "fixed-3" {
			f, e := os.Create(filepath.Join(dir, "live-registry-inuse.pprof"))
			if e != nil {
				t.Fatal(e)
			}
			if e = pprof.WriteHeapProfile(f); e != nil {
				t.Fatal(e)
			}
			f.Close()
		}
		runtime.KeepAlive(registry)
		runtime.KeepAlive(tool)
		runtime.KeepAlive(saved)
	}
	runtime.KeepAlive(saved)
	saved = nil
	registry = nil
	tool = nil
	runtime.GC()
	runtime.GC()
	var released runtime.MemStats
	runtime.ReadMemStats(&released)
	runtime.KeepAlive(saved)
	runtime.KeepAlive(tool)
	payload := struct {
		Windows      []memoryWindow `json:"Windows"`
		ReleasedHeap uint64         `json:"ReleasedHeap"`
		Note         string         `json:"Note"`
	}{checkpoints, released.HeapAlloc, "300 iterations/window; concurrent window has 8 calls/iteration; sampled heap after iterations; RSS not measured; finite diagnostic, not production budget"}
	raw, e := json.MarshalIndent(payload, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "memory.json"), raw, 0o600); e != nil {
		t.Fatal(e)
	}
}

func performanceToolForMemory(t *testing.T) Tool {
	t.Helper()
	schemaRaw, e := os.ReadFile("testdata/performance/zalo_list_groups.output.json")
	if e != nil {
		t.Fatal(e)
	}
	var schema map[string]any
	if e = json.Unmarshal(schemaRaw, &schema); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("testdata/performance/groups.payload.json")
	if e != nil {
		t.Fatal(e)
	}
	tool, e := NewProxyTool(
		"groups",
		"groups",
		[]byte(`{"type":"object"}`),
		func(_ context.Context, _ *RunEnv, _ []byte, y func(Chunk) error) error {
			return y(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: raw})
		},
		WithOutputSchema(schema),
	)
	if e != nil {
		t.Fatal(e)
	}
	return tool
}
