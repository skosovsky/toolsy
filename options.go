package toolsy

import (
	"context"
	"maps"
	"time"
)

// SchemaConfig contains JSON Schema generation settings for typed tools/extractors.
type SchemaConfig struct {
	Strict   bool
	Registry *SchemaRegistry
}

// ToolManifest contains metadata exposed to orchestrators and discovery layers.
type ToolManifest struct {
	Name        string
	Description string
	Parameters  map[string]any
	Timeout     time.Duration
	Tags        []string
	Version     string
	Metadata    map[string]any
}

// ToolConfig is the internal split configuration for a tool.
type ToolConfig struct {
	Schema   SchemaConfig
	Manifest ToolManifest
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
		c.Manifest.Timeout = d
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
		ensureManifestMetadata(&c.Manifest)
		c.Manifest.Metadata["dangerous"] = true
	}
}

// WithReadOnly marks the tool as read-only.
func WithReadOnly() ToolOption {
	return func(c *ToolConfig) {
		ensureManifestMetadata(&c.Manifest)
		c.Manifest.Metadata["read_only"] = true
	}
}

// WithMetadata merges custom metadata into the tool manifest metadata.
func WithMetadata(metadata map[string]any) ToolOption {
	return func(c *ToolConfig) {
		if len(metadata) == 0 {
			return
		}
		ensureManifestMetadata(&c.Manifest)
		maps.Copy(c.Manifest.Metadata, metadata)
	}
}

func ensureManifestMetadata(m *ToolManifest) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
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
// even on partial success or error). Summary reports delivered success chunks/bytes,
// delivered error chunks (soft errors), and final hard error.
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
