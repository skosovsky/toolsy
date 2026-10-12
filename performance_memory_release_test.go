package toolsy

import (
	"context"
	"runtime"
	"testing"
	"weak"
)

func TestMemoryOwnersReleased(t *testing.T) {
	// Arrange: cloned output remains alive, original graph and registry owners end.
	type node struct {
		Data [1024]byte
		Next *node
	}
	original, output := func() (weak.Pointer[node], *node) {
		source := &node{}
		source.Next = source
		return weak.Make(source), cloneMutableValue(source).(*node)
	}()
	registry := func() weak.Pointer[Registry] {
		tool := performanceToolForMemory(t)
		r, e := NewRegistryBuilder().Add(tool).Build()
		if e != nil {
			t.Fatal(e)
		}
		_ = cloneManifestForPolicy(tool.Manifest())
		return weak.Make(r)
	}()
	// Act: force collection after ending all owning scopes.
	for range 5 {
		runtime.GC()
	}
	// Assert: retained cloned output cannot keep original owners alive through scratch.
	if original.Value() != nil {
		t.Fatal("source retained after operation")
	}
	if registry.Value() != nil {
		t.Fatal("registry retained after release")
	}
	if output.Next != output {
		t.Fatal("retained snapshot cycle lost")
	}
	runtime.KeepAlive(output)
}

func TestMemoryPolicyRetainsSnapshots(t *testing.T) {
	// Arrange: policy retains complete request graphs across later executions.
	tool := performanceToolForMemory(t)
	var saved []PolicyRequest
	policy := PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
		saved = append(saved, req)
		return AllowDecision()
	})
	registry, err := NewRegistryBuilder(WithPolicy("retain", policy)).Add(tool).Build()
	if err != nil {
		t.Fatal(err)
	}
	call := ToolCall{ToolName: "groups", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	// Act: earlier callbacks keep snapshots while subsequent calls reuse scratch.
	for range 20 {
		if err = registry.Execute(context.Background(), call, func(Chunk) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	properties := saved[0].Manifest.OutputSchema["properties"].(map[string]any)
	properties["poison"] = map[string]any{"type": "boolean"}
	saved[0].Input.ArgsJSON[0] = '!'
	if err = registry.Execute(context.Background(), call, func(Chunk) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Assert: retained callbacks cannot change another call or constructor schema.
	for _, req := range saved[1:] {
		if req.Manifest.OutputSchema["properties"].(map[string]any)["poison"] != nil || req.Input.ArgsJSON[0] != '{' {
			t.Fatal("retained callback poisoned another request")
		}
	}
	if tool.Manifest().OutputSchema["properties"].(map[string]any)["poison"] != nil {
		t.Fatal("retained callback poisoned tool")
	}
}
