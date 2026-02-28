package toolsy

import (
	"encoding/json"
	"maps"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// Extractor provides JSON Schema generation and two-layer validation (schema + Validatable)
// for type T without binding to the Tool interface. Use it in custom orchestrators that need
// schema export and validated parsing but not the standard Execute([]byte) ([]byte, error) pipeline.
type Extractor[T any] struct {
	schemaMap map[string]any
	resolved  *jsonschema.Resolved
}

// NewExtractor creates an Extractor for type T. When strict is true, the generated schema
// has additionalProperties: false for all objects and all properties required (OpenAI Structured Outputs).
func NewExtractor[T any](strict bool) (*Extractor[T], error) {
	schemaMap, resolved, err := generateSchema[T](strict)
	if err != nil {
		return nil, err
	}
	return &Extractor[T]{
		schemaMap: schemaMap,
		resolved:  resolved,
	}, nil
}

// Schema returns a shallow copy of the JSON Schema (top-level keys only).
// Nested maps are shared; callers must not mutate them.
func (e *Extractor[T]) Schema() map[string]any {
	return maps.Clone(e.schemaMap)
}

// ParseAndValidate deserializes argsJSON into T, runs Layer 1 (schema validation) and
// Layer 2 (Validatable.Validate() if T implements it). Returns ClientError for invalid
// JSON or validation failures so the caller can pass the message to the LLM for self-correction.
func (e *Extractor[T]) ParseAndValidate(argsJSON []byte) (T, error) {
	var zero T
	var v any
	if err := json.Unmarshal(argsJSON, &v); err != nil {
		return zero, wrapJSONParseError(err)
	}
	if err := validateAgainstSchema(e.resolved, v); err != nil {
		return zero, err
	}
	var args T
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return zero, wrapJSONParseError(err)
	}
	// Layer 2: Validatable. Try args first (value receiver or T is *SomeType), then &args only
	// for value type T when args does not implement Validatable (pointer receiver).
	if err := runLayer2Validation(args); err != nil {
		if IsClientError(err) {
			return zero, err
		}
		return zero, &ClientError{Reason: err.Error(), Err: ErrValidation}
	}
	return args, nil
}

// runLayer2Validation runs Validatable.Validate() on args; if args does not implement Validatable,
// it tries &args for value types (pointer receiver). Never calls Validate twice for the same receiver.
func runLayer2Validation[T any](args T) error {
	if err := validateCustom(any(args)); err != nil {
		return err
	}
	if _, ok := any(args).(Validatable); ok {
		return nil
	}
	typ := reflect.TypeOf(args)
	if typ == nil || typ.Kind() == reflect.Pointer {
		return nil
	}
	return validateCustom(any(&args))
}
