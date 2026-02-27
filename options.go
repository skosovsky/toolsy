package toolsy

import (
	"context"
	"time"
)

// toolOptions hold optional tool settings (timeout, strict, tags, etc.).
type toolOptions struct {
	strict    bool
	timeout   time.Duration
	tags      []string
	version   string
	dangerous bool
}

// ToolOption configures a tool (e.g. WithStrict, WithTimeout).
type ToolOption func(*toolOptions)

// WithStrict sets strict mode for schema: additionalProperties: false for all objects,
// and all properties become required. Use for OpenAI Structured Outputs compatibility.
func WithStrict() ToolOption {
	return func(o *toolOptions) {
		o.strict = true
	}
}

// WithTimeout sets a per-tool timeout (stored in toolOptions for use by middleware or registry).
func WithTimeout(d time.Duration) ToolOption {
	return func(o *toolOptions) {
		o.timeout = d
	}
}

// WithTags sets tool tags (metadata for discovery/orchestrator).
func WithTags(tags ...string) ToolOption {
	return func(o *toolOptions) {
		o.tags = tags
	}
}

// WithVersion sets the tool version.
func WithVersion(version string) ToolOption {
	return func(o *toolOptions) {
		o.version = version
	}
}

// WithDangerous marks the tool as dangerous (orchestrator may require confirmation).
func WithDangerous() ToolOption {
	return func(o *toolOptions) {
		o.dangerous = true
	}
}

// RegistryOption configures a Registry.
type RegistryOption func(*registryOptions)

type registryOptions struct {
	timeout        time.Duration
	maxConcurrency int
	recoverPanics  bool
	onBefore       func(context.Context, ToolCall)
	onAfter        func(context.Context, ToolCall, ToolResult, time.Duration)
}

// WithDefaultTimeout sets the default execution timeout for tools.
func WithDefaultTimeout(d time.Duration) RegistryOption {
	return func(o *registryOptions) {
		o.timeout = d
	}
}

// WithMaxConcurrency limits concurrent tool executions (semaphore).
// Pass 0 or negative to disable the semaphore (unlimited concurrency).
func WithMaxConcurrency(n int) RegistryOption {
	return func(o *registryOptions) {
		o.maxConcurrency = n
	}
}

// WithRecoverPanics enables panic recovery in Execute (returns SystemError).
func WithRecoverPanics(enable bool) RegistryOption {
	return func(o *registryOptions) {
		o.recoverPanics = enable
	}
}

// WithOnBeforeExecute sets a hook called before each tool execution.
func WithOnBeforeExecute(fn func(context.Context, ToolCall)) RegistryOption {
	return func(o *registryOptions) {
		o.onBefore = fn
	}
}

// WithOnAfterExecute sets a hook called after each tool execution.
func WithOnAfterExecute(fn func(context.Context, ToolCall, ToolResult, time.Duration)) RegistryOption {
	return func(o *registryOptions) {
		o.onAfter = fn
	}
}
