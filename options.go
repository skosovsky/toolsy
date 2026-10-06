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
	StreamSemantics StreamSemantics
	StreamMaxBytes  int
	Name            string
	Description     string
	Parameters      map[string]any
	OutputSchema    map[string]any
	Tags            []string
	Version         string
	Requirements    ToolRequirements

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
// Enforcement belongs to registry/session policy (see [NewRequirementsPolicy]).
func WithRequirements(req ToolRequirements) ToolOption {
	return func(c *ToolConfig) {
		c.Manifest.Requirements = cloneRequirements(req)
	}
}

// WithOutputSchema sets the executable JSON Schema for successful JSON wire results.
// Builders compile it at construction and validate before persistence and delivery.
// It does not validate progress, controls, business errors, empty/noop or text/binary results.
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
	executionProfile ExecutionProfile
	recoverPanics    bool
	validator        Validator
	policy           Policy
	policyDigest     string
	policyIDMissing  bool
	policyInvalid    bool
	view             RegistryViewSnapshot
	onBefore         func(context.Context, ToolCall)
	onAfter          func(context.Context, ToolCall, ExecutionSummary, time.Duration)
	onChunk          func(context.Context, Chunk)
}

// WithRecoverPanics enables panic recovery in Execute (returns [ToolError] with [CodeInternal]).
func WithRecoverPanics(enable bool) RegistryOption {
	return func(o *registryOptions) {
		o.recoverPanics = enable
	}
}

// WithValidator configures a low-level reject-only validator run before tool unmarshaling (fail-closed).
//
// Use [ArgsBinder] through [NewTypedTool] or [NewPolicyTool] when validation
// needs to return canonical typed/sanitized args to the handler.
func WithValidator(v Validator) RegistryOption {
	return func(o *registryOptions) {
		o.validator = v
	}
}

// WithPolicy configures fail-closed policy/capability enforcement before tool handlers run.
// Explicit nil/typed-nil/nil-function policies fail Build; omission installs no gate.
// policyID is part of session/checkpoint binding and must change when policy semantics change.
func WithPolicy(policyID string, p Policy) RegistryOption {
	return func(o *registryOptions) {
		if isNilValue(p) {
			o.policyInvalid = true
			return
		}
		if policyID == "" {
			o.policyIDMissing = true
		}
		o.policy = composePolicies(o.policy, p)
		o.policyDigest = appendRegistryPolicyDigest(o.policyDigest, policyID)
	}
}

// WithRequirementsPolicy configures fail-closed typed enforcement for manifest requirements.
// policyID is part of session/checkpoint binding and must change when policy semantics change.
func WithRequirementsPolicy[TSubject, TScope any](
	policyID string,
	fn RequirementsDecisionFunc[TSubject, TScope],
) RegistryOption {
	if fn == nil {
		return WithPolicy(policyID, nil)
	}
	return WithPolicy(policyID, NewRequirementsPolicy(fn))
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
	maxCalls          int
	policy            RunPolicy
	codecRegistry     *StateCodecRegistry
	strictStateCodecs bool
}

// WithStateCodecRegistry supplies a codec builder for snapshot encode/decode.
// NewSession finalizes registration by freezing this shared registry after
// constructor validation. Configure all slots before creating any session.
func WithStateCodecRegistry(r *StateCodecRegistry) SessionOption {
	return func(o *sessionOptions) {
		o.codecRegistry = r
	}
}

// WithStrictStateCodecs requires every imported/exported session state key to have a registered [StateCodec].
// Empty snapshots are allowed; unknown JSON null values fail because there is no slot policy to authorize clearing.
func WithStrictStateCodecs(strict bool) SessionOption {
	return func(o *sessionOptions) {
		o.strictStateCodecs = strict
	}
}

// WithMaxCalls limits outer session call admissions. Zero is unlimited; negatives
// fail NewSession. CallAttempts also includes attempts rejected by this limit.
func WithMaxCalls(n int) SessionOption {
	return func(o *sessionOptions) {
		o.maxCalls = n
	}
}

// WithRunPolicy attaches session-level tool choice constraints enforced before each Execute.
func WithRunPolicy(p RunPolicy) SessionOption {
	p = cloneRunPolicy(p)
	return func(o *sessionOptions) {
		o.policy = cloneRunPolicy(p)
	}
}
