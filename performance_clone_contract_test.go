package toolsy

import (
	"context"
	"reflect"
	"testing"
)

func TestPerformanceCloneMixedMemo(t *testing.T) {
	// Arrange: aliases cross exact JSON and named BYOT containers in both directions.
	type named map[string]any
	type node struct {
		JSON    map[string]any
		Values  []any
		private map[string]any
	}
	root := map[string]any{}
	list := []any{root, nil}
	n := &node{JSON: root, Values: list, private: root}
	byot := named{"root": root, "list": list}
	root["node"], root["named"], root["list"], root["self"] = n, byot, list, root
	list[1] = byot
	// Act.
	cloned := cloneMutableValue(root).(map[string]any)
	cloned["changed"] = true
	cn := cloned["node"].(*node)
	cn.JSON["via_node"] = true
	cloned["named"].(named)["via_named"] = true
	// Assert.
	if root["changed"] != nil || root["via_node"] != nil || byot["via_named"] != nil {
		t.Fatal("source mutated")
	}
	if cloned["via_node"] != true || cloned["list"].([]any)[1].(named)["via_named"] != true {
		t.Fatal("mixed graph alias lost")
	}
	if reflect.ValueOf(cloned["self"]).Pointer() != reflect.ValueOf(cloned).Pointer() {
		t.Fatal("cycle lost")
	}
	if reflect.ValueOf(cn.private).Pointer() != reflect.ValueOf(root).Pointer() {
		t.Fatal("opaque private field changed")
	}
	if cap(cn.Values) != len(list) {
		t.Fatal("slice capacity changed")
	}
}

func TestPerformanceManifestNestedIsolation(t *testing.T) {
	// Arrange.
	schema := map[string]any{
		"type": "object", "required": []any{"x"},
		"properties": map[string]any{"x": map[string]any{"type": "string", "enum": []any{"allowed"}}},
	}
	base, err := NewProxyTool(
		"base",
		"base",
		[]byte(`{"type":"object"}`),
		func(context.Context, *RunEnv, []byte, func(Chunk) error) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	override := OverrideTool(base, WithNewParameters(schema))
	set, err := ManifestSetFromManifests(ToolManifest{Name: "base", Parameters: schema, OutputSchema: schema})
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	schema["properties"].(map[string]any)["x"].(map[string]any)["type"] = "boolean"
	first := override.Manifest()
	first.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] = "integer"
	m, _ := set.Manifest("base")
	m.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] = "number"
	m.Parameters["required"].([]any)[0] = "corrupt"
	m.OutputSchema["properties"].(map[string]any)["x"].(map[string]any)["enum"].([]any)[0] = "corrupt"
	first.Parameters["required"].([]any)[0] = "corrupt"
	// Assert.
	next := override.Manifest()
	if next.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] != "string" {
		t.Fatal("override schema escaped")
	}
	if next.Parameters["required"].([]any)[0] != "x" {
		t.Fatal("override nested slice escaped")
	}
	next, _ = set.Manifest("base")
	if next.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] != "string" ||
		next.OutputSchema["properties"].(map[string]any)["x"].(map[string]any)["type"] != "string" {
		t.Fatal("manifest set schema escaped")
	}
	if next.Parameters["required"].([]any)[0] != "x" ||
		next.OutputSchema["properties"].(map[string]any)["x"].(map[string]any)["enum"].([]any)[0] != "allowed" {
		t.Fatal("manifest set nested slice escaped")
	}
}

func FuzzPerformanceClone(f *testing.F) {
	// Arrange.
	f.Add("seed", true)
	f.Fuzz(func(t *testing.T, value string, cycle bool) {
		src := map[string]any{"value": value}
		shared := []any{src, src}
		src["list"] = shared
		if cycle {
			src["self"] = src
		}
		// Act.
		out := cloneMutableValue(src).(map[string]any)
		out["value"] = "changed"
		// Assert.
		if src["value"] != value {
			t.Fatal("source mutation")
		}
		for _, item := range out["list"].([]any) {
			if item.(map[string]any)["value"] != "changed" {
				t.Fatal("alias lost")
			}
		}
	})
}
