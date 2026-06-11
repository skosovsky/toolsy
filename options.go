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
	Name         string
	Description  string
	Parameters   map[string]any
	OutputSchema map[string]any
	Tags         []string
	Version      string
	Requirements ToolRequirements

	CompletionPolicy     CompletionPolicy
	ReadOnly             bool
	RequiresConfirmation bool
	Dangerous            bool
	Idempotent           bool
}

// ToolConfig is the internal split configuration for a tool.
type ToolConfig struct {
	Schema   SchemaConfig
	Manifest ToolManifest
}

// ToolOption configures a tool (e.g. WithStrict, WithSchemaRegistry).
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
		c.Manifest.Dangerous = true
	}
}

// WithReadOnly marks the tool as read-only.
func WithReadOnly() ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.ReadOnly = true
	}
}

// WithRequiresConfirmation marks the tool as requiring human confirmation before execution.
func WithRequiresConfirmation() ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.RequiresConfirmation = true
	}
}

// WithIdempotent marks mutating tools as safe to retry with identical arguments.
func WithIdempotent() ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Idempotent = true
	}
}

// WithRequirements sets typed declarative requirements on the tool manifest.
// Enforcement is the host's responsibility (see [ToolRequirements]).
func WithRequirements(req ToolRequirements) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Requirements = cloneRequirements(req)
	}
}

// WithOutputSchema sets the JSON Schema for tool results exposed to orchestrators.
func WithOutputSchema(schema map[string]any) ToolOption {
	return func(c *ToolConfig) {
		if len(schema) == 0 {
			c.Manifest.OutputSchema = nil
			return
		}
		c.Manifest.OutputSchema = maps.Clone(schema)
	}
}

// RegistryOption configures a Registry.
type RegistryOption func(*registryOptions)

type registryOptions struct {
	recoverPanics bool
	validator     Validator
	authorizer    Authorizer
	onBefore      func(context.Context, ToolCall)
	onAfter       func(context.Context, ToolCall, ExecutionSummary, time.Duration)
	onChunk       func(context.Context, Chunk)
}

// WithRecoverPanics enables panic recovery in Execute (returns [ToolError] with [CodeInternal]).
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
	maxSteps      int
	policy        RunPolicy
	codecRegistry *StateCodecRegistry
}

// WithStateCodecRegistry configures typed encode/decode for [Session.ExportSnapshot] and [Session.ImportSnapshot].
func WithStateCodecRegistry(r *StateCodecRegistry) SessionOption {
	return func(o *sessionOptions) {
		o.codecRegistry = r
	}
}

// WithMaxSteps limits the total number of tool executions within a session track.
func WithMaxSteps(n int) SessionOption {
	return func(o *sessionOptions) {
		o.maxSteps = n
	}
}

// WithRunPolicy attaches session-level tool choice constraints enforced before each Execute.
func WithRunPolicy(p RunPolicy) SessionOption {
	return func(o *sessionOptions) {
		o.policy = p
	}
}
