package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/skosovsky/toolsy/textprocessor"
)

// tool is the internal implementation of Tool built by NewTool, NewStreamTool, NewDynamicToolFromSpec, or NewProxyTool.
type tool struct {
	manifest ToolManifest
	execute  func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error
}

// NewTool builds a Tool from a typed function that also receives [*RunEnv].
func NewTool[T any, R any](
	name, description string,
	fn func(ctx context.Context, env *RunEnv, args T) (R, error),
	opts ...ToolOption,
) (Tool, error) {
	var cfg ToolConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.Schema = ensureSchemaConfig(cfg.Schema)
	ext, err := NewExtractorWithConfig[T](cfg.Schema)
	if err != nil {
		return nil, err
	}
	if len(cfg.Manifest.OutputSchema) == 0 {
		outSchema, genErr := generateOutputSchema[R](cfg.Schema)
		if genErr != nil {
			return nil, genErr
		}
		cfg.Manifest.OutputSchema = outSchema
	}
	execute := func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		args, err := ext.ParseAndValidate(input.ArgsJSON)
		if err != nil {
			return err
		}
		res, err := fn(ctx, env, args)
		if err != nil {
			return wrapHandlerError(err)
		}
		data, err := marshalToolResult(res)
		if err != nil {
			return NewInternalError(fmt.Errorf("toolsy: marshal typed result: %w", err))
		}
		chunk := Chunk{
			Event:       EventResult,
			Data:        data,
			MimeType:    MimeTypeJSON,
			TypedResult: res,
		}
		prepared, err := prepareChunk(chunk)
		if err != nil {
			return err
		}
		if err := yield(prepared); err != nil {
			return wrapYieldError(err)
		}
		return nil
	}
	return &tool{
		manifest: buildToolManifest(name, description, ext.Schema(), cfg.Manifest),
		execute:  execute,
	}, nil
}

// WireJSONResult is implemented by tool handler results that are already JSON-encoded for the wire.
// Used by toolkit IoC formatters after CapWireJSON; bytes may be truncated and not valid JSON.
type WireJSONResult interface {
	WireJSON() json.RawMessage
}

func marshalToolResult(res any) ([]byte, error) {
	if wr, ok := res.(WireJSONResult); ok {
		raw := wr.WireJSON()
		if raw == nil {
			return []byte("null"), nil
		}
		out := make([]byte, len(raw))
		copy(out, raw)
		return out, nil
	}
	return json.Marshal(res)
}

func generateOutputSchema[R any](cfg SchemaConfig) (map[string]any, error) {
	schemaMap, _, err := generateSchema[R](cfg)
	return schemaMap, err
}

// deepCopySchemaFromMap returns a defensive deep copy of schemaMap for mutation (strict mode, strip IDs).
func deepCopySchemaFromMap(schemaMap map[string]any) (map[string]any, error) {
	data, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("failed to deep copy schema map: %w", err)
	}
	var schemaCopy map[string]any
	if err := json.Unmarshal(data, &schemaCopy); err != nil {
		return nil, fmt.Errorf("failed to deep copy schema map: %w", err)
	}
	return schemaCopy, nil
}

// rawArgsValidatedExecute builds the execute closure shared by NewProxyTool:
// unmarshal args, validate against compiled schema, then run handler with yield wrapping.
//
//nolint:gocognit
func rawArgsValidatedExecute(
	compiled schemaValidator,
	handler func(ctx context.Context, env *RunEnv, argsJSON []byte, yield func(Chunk) error) error,
) func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
	return func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		var v any
		if err := json.Unmarshal(input.ArgsJSON, &v); err != nil {
			return wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return err
		}
		yieldWrapped := func(c Chunk) error {
			prepared, err := prepareChunk(c)
			if err != nil {
				return err
			}
			if err := yield(prepared); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		if err := handler(ctx, env, input.ArgsJSON, yieldWrapped); err != nil {
			if clientCorrectable(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			if IsControlError(err) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
}

// NewStreamTool builds a Tool from a typed streaming function that also receives [*RunEnv].
//
// Unlike [NewTool], stream tools do not have a single typed result type R, so
// [ToolManifest.OutputSchema] is not generated automatically. Set it with
// [WithOutputSchema] when the LLM should know the shape of final JSON results,
// or document progress/result chunks in the tool description.
//
//nolint:gocognit
func NewStreamTool[T any](
	name, description string,
	fn func(ctx context.Context, env *RunEnv, args T, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var cfg ToolConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.Schema = ensureSchemaConfig(cfg.Schema)
	ext, err := NewExtractorWithConfig[T](cfg.Schema)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		yieldWrapped := func(c Chunk) error {
			prepared, err := prepareChunk(c)
			if err != nil {
				return err
			}
			if err := yield(prepared); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		args, err := ext.ParseAndValidate(input.ArgsJSON)
		if err != nil {
			return err
		}
		if err := fn(ctx, env, args, yieldWrapped); err != nil {
			if clientCorrectable(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			if IsControlError(err) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
	return &tool{
		manifest: buildToolManifest(name, description, ext.Schema(), cfg.Manifest),
		execute:  execute,
	}, nil
}

// NewProxyTool creates a Tool from a raw JSON Schema (e.g. from an MCP server) and a handler that receives
// validated raw args and yield func(Chunk) error. No Go struct reflection; schema is used only for validation.
// rawJSONSchema and handler must be non-nil.
func NewProxyTool(
	name, description string,
	rawJSONSchema []byte,
	handler func(ctx context.Context, env *RunEnv, rawArgs []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var cfg ToolConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(rawJSONSchema) == 0 {
		return nil, errors.New("proxy schema must not be empty")
	}
	if handler == nil {
		return nil, errors.New("proxy tool handler must not be nil")
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawJSONSchema, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse proxy schema: %w", err)
	}
	schemaCopy, err := deepCopySchemaFromMap(parsed)
	if err != nil {
		return nil, fmt.Errorf("failed to copy proxy schema: %w", err)
	}
	if cfg.Schema.Strict {
		applyStrictMode(schemaCopy)
	}
	stripSchemaIDs(schemaCopy)
	compiled, err := compileRawSchema(schemaCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to compile proxy schema: %w", err)
	}
	execute := rawArgsValidatedExecute(compiled, handler)
	return &tool{
		manifest: buildToolManifest(name, description, schemaCopy, cfg.Manifest),
		execute:  execute,
	}, nil
}

func buildToolManifest(name, description string, schema map[string]any, cfg ToolManifest) ToolManifest {
	tags := append([]string(nil), cfg.Tags...)
	return ToolManifest{
		Name:                 name,
		Description:          description,
		Parameters:           maps.Clone(schema),
		OutputSchema:         maps.Clone(cfg.OutputSchema),
		Tags:                 tags,
		Version:              cfg.Version,
		Requirements:         cloneRequirements(cfg.Requirements),
		CompletionPolicy:     cfg.CompletionPolicy,
		ReadOnly:             cfg.ReadOnly,
		RequiresConfirmation: cfg.RequiresConfirmation,
		Dangerous:            cfg.Dangerous,
		Idempotent:           cfg.Idempotent,
	}
}

func (t *tool) Manifest() ToolManifest {
	m := t.manifest
	m.Tags = append([]string(nil), t.manifest.Tags...)
	m.Parameters = maps.Clone(t.manifest.Parameters)
	m.OutputSchema = maps.Clone(t.manifest.OutputSchema)
	m.Requirements = cloneRequirements(t.manifest.Requirements)
	m.CompletionPolicy = t.manifest.CompletionPolicy
	m.ReadOnly = t.manifest.ReadOnly
	m.RequiresConfirmation = t.manifest.RequiresConfirmation
	m.Dangerous = t.manifest.Dangerous
	m.Idempotent = t.manifest.Idempotent
	return m
}

func (t *tool) Execute(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
	if env == nil {
		env = NewRunEnv(nil)
	}
	if len(input.Attachments) > 0 {
		env = env.cloneForExecute(input.Attachments, env.async)
	}
	return t.execute(ctx, env, input, yield)
}

// wrapHandlerError passes through [ToolError] and control errors; wraps other errors as internal [ToolError].
func wrapHandlerError(err error) error {
	if err == nil {
		return nil
	}
	if IsControlError(err) {
		return err
	}
	if te, ok := AsToolError(err); ok && te.Code == CodeInternal && isContextInterrupt(te.Err) {
		err = te.Err
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewTimeoutErrorFrom(err, true)
	}
	if errors.Is(err, ErrTimeout) {
		return NewTimeoutErrorFrom(err, true)
	}
	if isContextInterrupt(err) {
		return err
	}
	if textprocessor.IsReadLimitExceeded(err) {
		if mapped := MapSandboxReadLimitError(err); mapped != nil {
			return mapped
		}
		return MapReadLimitError(err, 0)
	}
	if te, ok := AsToolError(err); ok {
		return te
	}
	return NewInternalError(err)
}

var _ Tool = (*tool)(nil)
