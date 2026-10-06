package toolsy

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// SchemaProvider supplies a JSON Schema map for dynamic tools.
type SchemaProvider interface {
	ParametersSchema() map[string]any
}

// MapSchemaProvider wraps a schema map as [SchemaProvider].
type MapSchemaProvider map[string]any

// ParametersSchema returns a defensive copy of the schema map.
func (m MapSchemaProvider) ParametersSchema() map[string]any {
	if len(m) == 0 {
		return nil
	}
	return maps.Clone(map[string]any(m))
}

// DynamicToolSpec describes a dynamic tool with schema validation and decoded args.
// Numbers in ValidateArgs/Handler maps are [json.Number]. Duplicate keys and input
// values above depth 128 or 100,000 nodes are rejected before dispatch.
type DynamicToolSpec struct {
	Name, Description string
	Schema            SchemaProvider
	ValidateArgs      func(ctx context.Context, decoded map[string]any) error
	Handler           func(ctx context.Context, env *RunEnv, decoded map[string]any, yield func(Chunk) error) error
	Options           []ToolOption
}

// NewDynamicToolFromSpec creates a [Tool] from [DynamicToolSpec].
//
//nolint:gocognit // schema compile + validated handler pipeline
func NewDynamicToolFromSpec(spec DynamicToolSpec) (Tool, error) {
	if spec.Handler == nil {
		return nil, ErrToolHandlerNil
	}
	if spec.Schema == nil {
		return nil, errors.New("dynamic tool schema provider must not be nil")
	}
	schemaMap := spec.Schema.ParametersSchema()
	if schemaMap == nil {
		return nil, errors.New("dynamic schema map must not be nil")
	}

	var cfg ToolConfig
	for _, opt := range spec.Options {
		opt(&cfg)
	}
	cfg.Schema = ensureSchemaConfig(cfg.Schema)

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

	validateArgs := spec.ValidateArgs
	handler := spec.Handler
	manifest := buildToolManifest(spec.Name, spec.Description, schemaCopy, cfg.Manifest)
	outputValidator, err := compileResultContract(&manifest)
	if err != nil {
		return nil, err
	}

	execute := func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		v, err := jsonschemax.Decode(input.ArgsJSON)
		if err != nil {
			return wrapJSONParseError(err)
		}
		if err := validateAgainstSchema(compiled, v); err != nil {
			return err
		}
		decoded, ok := v.(map[string]any)
		if !ok {
			return NewSchemaError("dynamic tool arguments must be a JSON object")
		}
		if validateArgs != nil {
			if vErr := runDynamicValidateArgs(ctx, validateArgs, decoded); vErr != nil {
				return vErr
			}
		}
		return executePreparedResult(
			ctx,
			env,
			manifest,
			input,
			decoded,
			outputValidator,
			func(_ ToolInput, out func(Chunk) error) error {
				yieldWrapped := func(c Chunk) error {
					if err := out(c); err != nil {
						return wrapYieldError(err)
					}
					return nil
				}
				if err := handler(ctx, env, decoded, yieldWrapped); err != nil {
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
			},
			yield,
		)
	}

	return &tool{
		manifest: manifest,
		execute:  execute,
	}, nil
}

func runDynamicValidateArgs(
	ctx context.Context,
	validateArgs func(context.Context, map[string]any) error,
	decoded map[string]any,
) error {
	if err := validateArgs(ctx, decoded); err != nil {
		if clientCorrectable(err) {
			return err
		}
		if _, ok := AsToolError(err); ok {
			return err
		}
		return NewValidationError(err.Error())
	}
	return nil
}
