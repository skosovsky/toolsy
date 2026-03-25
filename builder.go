package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"
)

// tool is the internal implementation of Tool built by NewTool, NewStreamTool, NewDynamicTool, or NewProxyTool.
type tool struct {
	name        string
	description string
	schema      map[string]any
	execute     func(context.Context, RunContext, []byte, func(Chunk) error) error
	config      ToolConfig
}

// NewToolWithRun builds a Tool from a typed function that also receives RunContext.
func NewToolWithRun[T any, R any](
	name, description string,
	fn func(ctx context.Context, run RunContext, args T) (R, error),
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
	execute := func(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error {
		args, err := ext.ParseAndValidate(argsJSON)
		if err != nil {
			return err
		}
		res, err := fn(ctx, run, args)
		if err != nil {
			return wrapHandlerError(err)
		}
		chunk, err := normalizeChunk(Chunk{Event: EventResult, RawData: res})
		if err != nil {
			return err
		}
		if err := yield(chunk); err != nil {
			return wrapYieldError(err)
		}
		return nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      ext.Schema(),
		execute:     execute,
		config:      cfg,
	}, nil
}

// NewTool builds a Tool from a typed function. Convenience wrapper over NewToolWithRun with an empty RunContext.
func NewTool[T any, R any](
	name, description string,
	fn func(ctx context.Context, args T) (R, error),
	opts ...ToolOption,
) (Tool, error) {
	return NewToolWithRun(name, description, func(ctx context.Context, _ RunContext, args T) (R, error) {
		return fn(ctx, args)
	}, opts...)
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

// rawArgsValidatedExecute builds the execute closure shared by NewDynamicTool and NewProxyTool:
// unmarshal args, validate against compiled schema, then run handler with yield wrapping.
//
//nolint:gocognit
func rawArgsValidatedExecute(
	compiled schemaValidator,
	handler func(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error,
) func(context.Context, RunContext, []byte, func(Chunk) error) error {
	return func(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error {
		var v any
		if err := json.Unmarshal(argsJSON, &v); err != nil {
			return wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return err
		}
		yieldWrapped := func(c Chunk) error {
			normalized, err := normalizeChunk(c)
			if err != nil {
				return err
			}
			if err := yield(normalized); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		if err := handler(ctx, run, argsJSON, yieldWrapped); err != nil {
			if IsClientError(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			if errors.Is(err, ErrSuspend) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
}

// NewStreamToolWithRun builds a Tool from a typed streaming function that also receives RunContext.
//
//nolint:gocognit
func NewStreamToolWithRun[T any](
	name, description string,
	fn func(ctx context.Context, run RunContext, args T, yield func(Chunk) error) error,
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
	execute := func(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error {
		yieldWrapped := func(c Chunk) error {
			normalized, err := normalizeChunk(c)
			if err != nil {
				return err
			}
			if err := yield(normalized); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		args, err := ext.ParseAndValidate(argsJSON)
		if err != nil {
			return err
		}
		if err := fn(ctx, run, args, yieldWrapped); err != nil {
			if IsClientError(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			if errors.Is(err, ErrSuspend) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      ext.Schema(),
		execute:     execute,
		config:      cfg,
	}, nil
}

// NewStreamTool builds a Tool from a typed streaming function. Convenience wrapper over NewStreamToolWithRun.
func NewStreamTool[T any](
	name, description string,
	fn func(ctx context.Context, args T, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	return NewStreamToolWithRun(
		name,
		description,
		func(ctx context.Context, _ RunContext, args T, yield func(Chunk) error) error {
			return fn(ctx, args, yield)
		},
		opts...,
	)
}

// NewDynamicToolWithRun creates a Tool from a raw JSON Schema map and a streaming function that receives
// validated JSON and yield func(Chunk) error. Useful for runtime API integration (e.g. OpenAPI/Swagger). Layer 1
// (schema) validation only; handler receives raw []byte and may call yield zero or more times.
// schemaMap and fn must be non-nil. Error handling matches NewTool; yield errors become ErrStreamAborted.
// The provided schemaMap is not mutated; a defensive copy is made before any modifications (e.g. WithStrict).
func NewDynamicToolWithRun(
	name, description string,
	schemaMap map[string]any,
	fn func(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var cfg ToolConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if schemaMap == nil {
		return nil, errors.New("dynamic schema map must not be nil")
	}
	if fn == nil {
		return nil, errors.New("dynamic tool handler must not be nil")
	}
	schemaCopy, err := deepCopySchemaFromMap(schemaMap)
	if err != nil {
		return nil, err
	}
	if cfg.Schema.Strict {
		applyStrictMode(schemaCopy)
	}
	stripSchemaIDs(schemaCopy)
	compiled, err := compileRawSchema(schemaCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to compile dynamic schema: %w", err)
	}
	execute := rawArgsValidatedExecute(compiled, fn)
	return &tool{
		name:        name,
		description: description,
		schema:      schemaCopy,
		execute:     execute,
		config:      cfg,
	}, nil
}

// NewDynamicTool creates a Tool from a raw JSON Schema map. Convenience wrapper over NewDynamicToolWithRun.
func NewDynamicTool(
	name, description string,
	schemaMap map[string]any,
	fn func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	if fn == nil {
		return nil, errors.New("dynamic tool handler must not be nil")
	}
	return NewDynamicToolWithRun(
		name,
		description,
		schemaMap,
		func(ctx context.Context, _ RunContext, argsJSON []byte, yield func(Chunk) error) error {
			return fn(ctx, argsJSON, yield)
		},
		opts...,
	)
}

// NewProxyToolWithRun creates a Tool from a raw JSON Schema (e.g. from an MCP server) and a handler that receives
// validated raw args and yield func(Chunk) error. No Go struct reflection; schema is used only for validation.
// rawJSONSchema and handler must be non-nil. Parameters() returns the parsed schema (shallow copy).
func NewProxyToolWithRun(
	name, description string,
	rawJSONSchema []byte,
	handler func(ctx context.Context, run RunContext, rawArgs []byte, yield func(Chunk) error) error,
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
		name:        name,
		description: description,
		schema:      schemaCopy,
		execute:     execute,
		config:      cfg,
	}, nil
}

// NewProxyTool creates a Tool from a raw JSON Schema. Convenience wrapper over NewProxyToolWithRun.
func NewProxyTool(
	name, description string,
	rawJSONSchema []byte,
	handler func(ctx context.Context, rawArgs []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	if handler == nil {
		return nil, errors.New("proxy tool handler must not be nil")
	}
	return NewProxyToolWithRun(
		name,
		description,
		rawJSONSchema,
		func(ctx context.Context, _ RunContext, rawArgs []byte, yield func(Chunk) error) error {
			return handler(ctx, rawArgs, yield)
		},
		opts...,
	)
}

func (t *tool) Name() string        { return t.name }
func (t *tool) Description() string { return t.description }

// Parameters returns a shallow copy of the JSON Schema (top-level keys only).
// Nested maps (e.g. under "properties") are shared; callers must not mutate them.
func (t *tool) Parameters() map[string]any { return maps.Clone(t.schema) }

func (t *tool) Execute(ctx context.Context, run RunContext, argsJSON []byte, yield func(Chunk) error) error {
	return t.execute(ctx, run, argsJSON, yield)
}

func (t *tool) Timeout() time.Duration { return t.config.Execution.Timeout }
func (t *tool) Tags() []string         { return append([]string(nil), t.config.Manifest.Tags...) }
func (t *tool) Version() string        { return t.config.Manifest.Version }
func (t *tool) IsDangerous() bool      { return t.config.Execution.Dangerous }

func (t *tool) IsReadOnly() bool           { return t.config.Execution.ReadOnly }
func (t *tool) RequiresConfirmation() bool { return t.config.Manifest.RequiresConfirmation }
func (t *tool) Sensitivity() string        { return t.config.Manifest.Sensitivity }

// wrapHandlerError passes through ClientError; wraps other errors as SystemError.
func wrapHandlerError(err error) error {
	if err == nil {
		return nil
	}
	if IsClientError(err) {
		return err
	}
	if errors.Is(err, ErrSuspend) {
		return err
	}
	return &SystemError{Err: err}
}

var (
	_ Tool         = (*tool)(nil)
	_ ToolMetadata = (*tool)(nil)
)
