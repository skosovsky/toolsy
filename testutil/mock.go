// Package testutil provides test helpers for toolsy (e.g. MockTool).
package testutil

import (
	"context"

	"github.com/skosovsky/toolsy"
)

// MockTool is a configurable Tool implementation for tests.
type MockTool struct {
	NameVal   string
	DescVal   string
	ParamsVal map[string]any
	ExecuteFn func(ctx context.Context, args []byte, yield func(toolsy.Chunk) error) error
}

// Name returns the tool name.
func (m *MockTool) Name() string {
	if m.NameVal != "" {
		return m.NameVal
	}
	return "mock"
}

// Description returns the tool description.
func (m *MockTool) Description() string {
	return m.DescVal
}

// Parameters returns the parameters schema (or empty map).
func (m *MockTool) Parameters() map[string]any {
	if m.ParamsVal != nil {
		return m.ParamsVal
	}
	return map[string]any{}
}

// Execute runs ExecuteFn if set, otherwise returns nil.
func (m *MockTool) Execute(ctx context.Context, args []byte, yield func(toolsy.Chunk) error) error {
	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, args, yield)
	}
	return nil
}

// Ensure MockTool implements Tool.
var _ toolsy.Tool = (*MockTool)(nil)
