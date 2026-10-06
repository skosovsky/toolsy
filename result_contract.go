package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"reflect"
	"strings"
	"sync"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const resultInvalidJSON = "invalid_json"

// A custom encoder's wire shape cannot be inferred from its Go storage type.
func hasCustomResultEncoding[R any]() bool {
	t := reflect.TypeFor[R]()
	return t.Implements(reflect.TypeFor[WireJSONResult]()) || t.Implements(reflect.TypeFor[json.Marshaler]())
}

// ResultContractError reports an invalid tool output, not correctable arguments.
// A handler may already have caused effects; this error never authorizes retry.
// Kind identifies the wire failure or post-handler phase: result_validator,
// effect_validator or postcondition. Cause retains the original callback error.
type ResultContractError struct {
	Kind  string
	Cause error
}

func (e *ResultContractError) Error() string { return "toolsy: result " + e.Kind }
func (e *ResultContractError) Unwrap() error { return e.Cause }

// compileResultContract snapshots and compiles the advertised JSON output schema.
// An absent schema leaves the shape unrestricted, but JSON must still be valid.
func compileResultContract(manifest *ToolManifest) (schemaValidator, error) {
	if len(manifest.OutputSchema) == 0 {
		return nil, nil //nolint:nilnil // No schema means no shape validator.
	}
	schema, err := deepCopySchemaFromMap(manifest.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("toolsy: copy output schema: %w", err)
	}
	compiled, err := jsonschemax.Compile(schema)
	if err != nil {
		return nil, fmt.Errorf("toolsy: compile output schema: %w", err)
	}
	manifest.OutputSchema = schema
	return compiled, nil
}

func jsonMimeType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == MimeTypeJSON || strings.HasSuffix(mediaType, "+json")
}

func prepareResultChunk(c Chunk, validator schemaValidator) (Chunk, error) {
	prepared, err := prepareChunk(c)
	if err != nil {
		return Chunk{}, err
	}
	if c.Event != EventResult || c.IsError || c.EmptyResult || c.Noop {
		return prepared, nil
	}
	if len(c.Data) == 0 {
		if validator != nil {
			return Chunk{}, NewInternalError(&ResultContractError{Kind: "missing_value", Cause: nil})
		}
		return prepared, nil
	}
	if !jsonMimeType(c.MimeType) {
		return prepared, nil
	}
	value, err := jsonschemax.Decode(c.Data)
	if err != nil {
		return Chunk{}, NewInternalError(&ResultContractError{Kind: resultInvalidJSON, Cause: err})
	}
	if validator != nil {
		if err := validator.Validate(value); err != nil {
			return Chunk{}, NewInternalError(&ResultContractError{Kind: "schema_mismatch", Cause: err})
		}
	}
	return prepared, nil
}

// executePreparedResult validates before profile persistence and again on replay.
// A producer cannot suppress an output rejection by ignoring its yield error.
func executePreparedResult(
	ctx context.Context,
	env *RunEnv,
	manifest ToolManifest,
	input ToolInput,
	args any,
	validator schemaValidator,
	handler func(ToolInput, func(Chunk) error) error,
	yield func(Chunk) error,
) error {
	deliver := func(c Chunk) error {
		prepared, err := prepareResultChunk(c, validator)
		if err != nil {
			return err
		}
		return yield(prepared)
	}
	produce := func(bound ToolInput, out func(Chunk) error) error {
		var mu sync.Mutex
		var rejected error
		err := handler(bound, func(c Chunk) error {
			mu.Lock()
			prior := rejected
			mu.Unlock()
			if prior != nil {
				return prior
			}
			prepared, validationErr := prepareResultChunk(c, validator)
			if validationErr != nil {
				mu.Lock()
				if rejected == nil {
					rejected = validationErr
				}
				mu.Unlock()
				return validationErr
			}
			return out(prepared)
		})
		mu.Lock()
		defer mu.Unlock()
		if rejected != nil {
			return rejected
		}
		return err
	}
	return ExecutePrepared(ctx, env, manifest, input, args, produce, deliver)
}
