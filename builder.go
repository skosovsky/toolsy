package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"
)

// tool is the internal implementation of Tool built by NewTool.
type tool struct {
	name        string
	description string
	schema      map[string]any
	execute     func(context.Context, []byte) ([]byte, error)
	opts        toolOptions
}

// NewTool builds a Tool from a typed function. Schema and validation are delegated to Extractor[T];
// Execute runs ParseAndValidate then fn, then marshals the result.
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
	execute := func(ctx context.Context, argsJSON []byte) ([]byte, error) {
		args, err := ext.ParseAndValidate(argsJSON)
		if err != nil {
			return nil, err
		}
		result, err := fn(ctx, args)
		if err != nil {
			return nil, wrapHandlerError(err)
		}
		out, err := json.Marshal(result)
		if err != nil {
			return nil, &SystemError{Err: err}
		}
		return out, nil
	}
	return &tool{
		name:        name,
		description: description,
		schema:      ext.Schema(),
		execute:     execute,
		opts:        o,
	}, nil
}

// NewDynamicTool creates a Tool from a raw JSON Schema map and a function that receives validated JSON.
// Useful for runtime API integration (e.g. OpenAPI/Swagger). Only Layer 1 (schema) validation is applied;
// the handler receives raw []byte. schemaMap and fn must be non-nil. Error handling matches NewTool:
// ClientError is passed through, other errors are wrapped as SystemError. ToolOption (WithTimeout, WithStrict, etc.) is supported.
// The provided schemaMap is not mutated; a defensive copy is made before any modifications (e.g. WithStrict).
func NewDynamicTool(
	name, description string,
	schemaMap map[string]any,
	fn func(ctx context.Context, argsJSON []byte) ([]byte, error),
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
	execute := func(ctx context.Context, argsJSON []byte) ([]byte, error) {
		var v any
		if err := json.Unmarshal(argsJSON, &v); err != nil {
			return nil, wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return nil, err
		}
		res, err := fn(ctx, argsJSON)
		if err != nil {
			return nil, wrapHandlerError(err)
		}
		return res, nil
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

func (t *tool) Execute(ctx context.Context, argsJSON []byte) ([]byte, error) {
	return t.execute(ctx, argsJSON)
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
