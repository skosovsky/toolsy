package toolsy

import (
	"context"
	"time"
)

// ExecutionConfig contains runtime-only settings used when a tool executes.
type ExecutionConfig struct {
	Timeout   time.Duration
	Dangerous bool
	ReadOnly  bool
}

// SchemaConfig contains JSON Schema generation settings for typed tools/extractors.
type SchemaConfig struct {
	Strict   bool
	Registry *SchemaRegistry
}

// ToolManifest contains metadata exposed to orchestrators and discovery layers.
type ToolManifest struct {
	Tags                 []string
	Version              string
	RequiresConfirmation bool
	Sensitivity          string
}

// ToolConfig is the internal split configuration for a tool.
type ToolConfig struct {
	Execution ExecutionConfig
	Schema    SchemaConfig
	Manifest  ToolManifest
}

// ToolOption configures a tool (e.g. WithStrict, WithTimeout, WithSchemaRegistry).
type ToolOption func(*ToolConfig)

// WithStrict sets strict mode for schema: additionalProperties: false for all objects,
// and all properties become required. Use for OpenAI Structured Outputs compatibility.
func WithStrict() ToolOption {
	return func(c *ToolConfig) {
		c.Schema.Strict = true
	}
}

// WithSchemaRegistry configures the schema registry used for typed schema generation.
// When omitted, typed builders and extractors create an isolated registry automatically.
func WithSchemaRegistry(r *SchemaRegistry) ToolOption {
	return func(c *ToolConfig) {
		c.Schema.Registry = r
	}
}

// WithTimeout sets a per-tool timeout (used by middleware or registry execution).
func WithTimeout(d time.Duration) ToolOption {
	return func(c *ToolConfig) {
		c.Execution.Timeout = d
	}
}

// WithTags sets tool tags (metadata for discovery/orchestrator).
func WithTags(tags ...string) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Tags = append([]string(nil), tags...)
	}
}

// WithVersion sets the tool version.
func WithVersion(version string) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Version = version
	}
}

// WithDangerous marks the tool as dangerous.
func WithDangerous() ToolOption {
	return func(c *ToolConfig) {
		c.Execution.Dangerous = true
	}
}

// WithReadOnly marks the tool as read-only.
func WithReadOnly() ToolOption {
	return func(c *ToolConfig) {
		c.Execution.ReadOnly = true
	}
}

// WithRequiresConfirmation marks the tool as requiring human confirmation before execution.
func WithRequiresConfirmation() ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.RequiresConfirmation = true
	}
}

// WithSensitivity sets the sensitivity level metadata for the tool.
func WithSensitivity(level string) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Sensitivity = level
	}
}

// RegistryOption configures a Registry.
type RegistryOption func(*registryOptions)

type registryOptions struct {
	timeout        time.Duration
	maxConcurrency int
	recoverPanics  bool
	validator      Validator
	onBefore       func(context.Context, ToolCall)
	onAfter        func(context.Context, ToolCall, ExecutionSummary, time.Duration)
	onChunk        func(context.Context, Chunk)
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

// WithValidator configures a validator run before tool unmarshaling (fail-closed).
func WithValidator(v Validator) RegistryOption {
	return func(o *registryOptions) {
		o.validator = v
	}
}

// WithOnBeforeExecute sets a hook called before each tool execution.
func WithOnBeforeExecute(fn func(context.Context, ToolCall)) RegistryOption {
	return func(o *registryOptions) {
		o.onBefore = fn
	}
}

// WithOnAfterExecute sets a hook called after each tool execution (always invoked via defer,
// even on partial success or error). Summary reports chunks/bytes delivered and final error.
func WithOnAfterExecute(fn func(context.Context, ToolCall, ExecutionSummary, time.Duration)) RegistryOption {
	return func(o *registryOptions) {
		o.onAfter = fn
	}
}

// WithOnChunk sets a hook called for each non-error chunk successfully delivered (when yield returns nil). Observability only.
func WithOnChunk(fn func(context.Context, Chunk)) RegistryOption {
	return func(o *registryOptions) {
		o.onChunk = fn
	}
}

// SessionOption configures a Session.
type SessionOption func(*sessionOptions)

type sessionOptions struct {
	maxSteps int
}

// WithMaxSteps limits the total number of tool executions within a session track.
func WithMaxSteps(n int) SessionOption {
	return func(o *sessionOptions) {
		o.maxSteps = n
	}
}
