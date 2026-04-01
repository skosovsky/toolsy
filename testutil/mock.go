// Package testutil provides test helpers for toolsy (e.g. MockTool).
package testutil

import (
	"context"

	"github.com/skosovsky/toolsy"
)

// MockTool is a configurable Tool implementation for tests.
type MockTool struct {
	ManifestVal toolsy.ToolManifest
	ExecuteFn   func(
		ctx context.Context,
		run toolsy.RunContext,
		input toolsy.ToolInput,
		yield func(toolsy.Chunk) error,
	) error
}

// Manifest returns the tool manifest.
func (m *MockTool) Manifest() toolsy.ToolManifest {
	if m.ManifestVal.Name != "" {
		return m.ManifestVal
	}
	return toolsy.ToolManifest{
		Name:        "mock",
		Description: "",
		Parameters:  map[string]any{"type": "object"},
		Tags:        nil,
		Version:     "",
		Metadata:    nil,
	}
}

// Execute runs ExecuteFn if set, otherwise returns nil.
func (m *MockTool) Execute(
	ctx context.Context,
	run toolsy.RunContext,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, run, input, yield)
	}
	return nil
}

// Ensure MockTool implements Tool.
var _ toolsy.Tool = (*MockTool)(nil)
