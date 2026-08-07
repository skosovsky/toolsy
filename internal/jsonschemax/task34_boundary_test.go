package jsonschemax

import (
	"strings"
	"testing"
)

func TestDecodeRejectsRecursiveDuplicateKeys(t *testing.T) {
	// Arrange.
	raw := []byte(`{"properties":{"value":{"type":"string","type":"integer"}}}`)

	// Act.
	_, err := Decode(raw)

	// Assert.
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-key error, got %v", err)
	}
}

func TestCompileRejectsExcessiveSchemaDepth(t *testing.T) {
	// Arrange.
	var schema any = true
	for range maxSchemaDepth + 1 {
		schema = map[string]any{"not": schema}
	}

	// Act.
	_, err := Compile(schema)

	// Assert.
	if err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("expected depth error, got %v", err)
	}
}

func TestCompileAcceptsBoundedLocalReferenceCycle(t *testing.T) {
	// Arrange.
	schema := map[string]any{
		"$defs": map[string]any{
			"node": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"next": map[string]any{"$ref": "#/$defs/node"},
				},
			},
		},
		"$ref": "#/$defs/node",
	}

	// Act.
	compiled, err := Compile(schema)

	// Assert.
	if err != nil || compiled == nil {
		t.Fatalf("expected bounded local reference cycle to compile, schema=%v err=%v", compiled, err)
	}
}

func TestCompileRejectsFanOutAboveNodeBudget(t *testing.T) {
	// Arrange.
	branches := make([]any, maxSchemaNodes+1)
	for index := range branches {
		branches[index] = true
	}
	schema := map[string]any{"anyOf": branches}

	// Act.
	_, err := Compile(schema)

	// Assert.
	if err == nil || !strings.Contains(err.Error(), "maximum node count") {
		t.Fatalf("expected node-budget error, got %v", err)
	}
}
