package toolsy

import (
	"context"
	"encoding/json"
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

// NewTool builds a Tool from a typed function. Schema is generated from T (Layer 1);
// if T implements Validatable, Layer 2 is run after unmarshal.
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
	schemaMap, compiled, err := generateSchema[T](o.strict)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, argsJSON []byte) ([]byte, error) {
		var v any
		if err := json.Unmarshal(argsJSON, &v); err != nil {
			return nil, &ClientError{Reason: "json parse error: " + err.Error()}
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return nil, err
		}
		var args T
		// Same bytes already unmarshaled above; error here is effectively unreachable for valid JSON.
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return nil, &ClientError{Reason: "json parse error: " + err.Error()}
		}
		if err := validateCustom(any(&args)); err != nil {
			if IsClientError(err) {
				return nil, err
			}
			return nil, &ClientError{Reason: err.Error(), Err: ErrValidation}
		}
		result, err := fn(ctx, args)
		if err != nil {
			if IsClientError(err) {
				return nil, err
			}
			return nil, &SystemError{Err: err}
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
		schema:      schemaMap,
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

var (
	_ Tool         = (*tool)(nil)
	_ ToolMetadata = (*tool)(nil)
)
