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
	execute     func(context.Context, []byte, func(Chunk) error) error
	opts        toolOptions
}

// NewTool builds a Tool from a typed function. Schema and validation are delegated to Extractor[T].
// Execute runs ParseAndValidate, fn, marshals the result, then calls yield once with Chunk{Event: EventResult, Data: b}.
// If yield returns an error, it is returned as ErrStreamAborted (via wrapYieldError).
// Returns an error if schema generation fails (e.g. unsupported type).
func NewTool[T any, R any](
	name, description string,
	fn func(ctx context.Context, args T) (R, error),
	opts ...ToolOption,
) (Tool, error) {
	var o toolOptions
	for _, opt := range opts {
		opt(&o)
	}
	ext, err := NewExtractor[T](o.strict)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
		args, err := ext.ParseAndValidate(argsJSON)
		if err != nil {
			return err
		}
		res, err := fn(ctx, args)
		if err != nil {
			return wrapHandlerError(err)
		}
		b, err := json.Marshal(res)
		if err != nil {
			return &SystemError{Err: err}
		}
		if err := yield(Chunk{Event: EventResult, Data: b}); err != nil {
			return wrapYieldError(err)
		}
		return nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      ext.Schema(),
		execute:     execute,
		opts:        o,
	}, nil
}

// NewStreamTool builds a Tool from a typed streaming function. Same schema/validation as NewTool,
// but the handler receives yield func(Chunk) error and may call it multiple times. Zero chunks is valid (side-effects only).
// If yield returns an error, execution must stop and that error is returned as ErrStreamAborted.
func NewStreamTool[T any](
	name, description string,
	fn func(ctx context.Context, args T, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var o toolOptions
	for _, opt := range opts {
		opt(&o)
	}
	ext, err := NewExtractor[T](o.strict)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
		yieldWrapped := func(c Chunk) error {
			if err := yield(c); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		args, err := ext.ParseAndValidate(argsJSON)
		if err != nil {
			return err
		}
		if err := fn(ctx, args, yieldWrapped); err != nil {
			if IsClientError(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
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
		opts:        o,
	}, nil
}

// NewDynamicTool creates a Tool from a raw JSON Schema map and a streaming function that receives
// validated JSON and yield func(Chunk) error. Useful for runtime API integration (e.g. OpenAPI/Swagger). Layer 1
// (schema) validation only; handler receives raw []byte and may call yield zero or more times.
// schemaMap and fn must be non-nil. Error handling matches NewTool; yield errors become ErrStreamAborted.
// The provided schemaMap is not mutated; a defensive copy is made before any modifications (e.g. WithStrict).
func NewDynamicTool(
	name, description string,
	schemaMap map[string]any,
	fn func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var o toolOptions
	for _, opt := range opts {
		opt(&o)
	}
	if schemaMap == nil {
		return nil, fmt.Errorf("dynamic schema map must not be nil")
	}
	if fn == nil {
		return nil, fmt.Errorf("dynamic tool handler must not be nil")
	}
	// Defensive deep copy before any modifications so caller's map is never mutated.
	data, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("failed to deep copy schema map: %w", err)
	}
	var schemaCopy map[string]any
	if err := json.Unmarshal(data, &schemaCopy); err != nil {
		return nil, fmt.Errorf("failed to deep copy schema map: %w", err)
	}
	if o.strict {
		applyStrictMode(schemaCopy)
	}
	stripSchemaIDs(schemaCopy)
	compiled, err := compileRawSchema(schemaCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to compile dynamic schema: %w", err)
	}
	execute := func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
		var v any
		if err := json.Unmarshal(argsJSON, &v); err != nil {
			return wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return err
		}
		yieldWrapped := func(c Chunk) error {
			if err := yield(c); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		if err := fn(ctx, argsJSON, yieldWrapped); err != nil {
			if IsClientError(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      schemaCopy,
		execute:     execute,
		opts:        o,
	}, nil
}

// NewProxyTool creates a Tool from a raw JSON Schema (e.g. from an MCP server) and a handler that receives
// validated raw args and yield func(Chunk) error. No Go struct reflection; schema is used only for validation.
// rawJSONSchema and handler must be non-nil. Parameters() returns the parsed schema (shallow copy).
func NewProxyTool(
	name, description string,
	rawJSONSchema []byte,
	handler func(ctx context.Context, rawArgs []byte, yield func(Chunk) error) error,
	opts ...ToolOption,
) (Tool, error) {
	var o toolOptions
	for _, opt := range opts {
		opt(&o)
	}
	if len(rawJSONSchema) == 0 {
		return nil, fmt.Errorf("proxy schema must not be empty")
	}
	if handler == nil {
		return nil, fmt.Errorf("proxy tool handler must not be nil")
	}
	var schemaCopy map[string]any
	if err := json.Unmarshal(rawJSONSchema, &schemaCopy); err != nil {
		return nil, fmt.Errorf("failed to parse proxy schema: %w", err)
	}
	// Deep copy so we can mutate
	data, err := json.Marshal(schemaCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to copy proxy schema: %w", err)
	}
	if err := json.Unmarshal(data, &schemaCopy); err != nil {
		return nil, fmt.Errorf("failed to copy proxy schema: %w", err)
	}
	if o.strict {
		applyStrictMode(schemaCopy)
	}
	stripSchemaIDs(schemaCopy)
	compiled, err := compileRawSchema(schemaCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to compile proxy schema: %w", err)
	}
	execute := func(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
		var v any
		if err := json.Unmarshal(argsJSON, &v); err != nil {
			return wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return err
		}
		yieldWrapped := func(c Chunk) error {
			if err := yield(c); err != nil {
				return wrapYieldError(err)
			}
			return nil
		}
		if err := handler(ctx, argsJSON, yieldWrapped); err != nil {
			if IsClientError(err) {
				return err
			}
			if errors.Is(err, ErrStreamAborted) {
				return err
			}
			return wrapHandlerError(err)
		}
		return nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      schemaCopy,
		execute:     execute,
		opts:        o,
	}, nil
}

func (t *tool) Name() string        { return t.name }
func (t *tool) Description() string { return t.description }

// Parameters returns a shallow copy of the JSON Schema (top-level keys only).
// Nested maps (e.g. under "properties") are shared; callers must not mutate them.
func (t *tool) Parameters() map[string]any { return maps.Clone(t.schema) }

func (t *tool) Execute(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
	return t.execute(ctx, argsJSON, yield)
}

func (t *tool) Timeout() time.Duration { return t.opts.timeout }
func (t *tool) Tags() []string         { return append([]string(nil), t.opts.tags...) }
func (t *tool) Version() string        { return t.opts.version }
func (t *tool) IsDangerous() bool      { return t.opts.dangerous }

// wrapHandlerError passes through ClientError; wraps other errors as SystemError.
func wrapHandlerError(err error) error {
	if err == nil {
		return nil
	}
	if IsClientError(err) {
		return err
	}
	return &SystemError{Err: err}
}

var (
	_ Tool         = (*tool)(nil)
	_ ToolMetadata = (*tool)(nil)
)
