package toolsy

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"

	invopop "github.com/invopop/jsonschema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	customTypesMu sync.RWMutex
	customTypes   = make(map[reflect.Type]*invopop.Schema)
)

// RegisterType registers a custom Go type to be mapped to a JSON Schema type/format in generated schemas.
// emptyInstance is a value of the type to register (e.g. uuid.UUID{}, or MyMoney{}); it must not be nil.
// jsonType is the JSON Schema type (e.g. "string", "number"); it must not be empty.
// format is optional (e.g. "uuid", "decimal"). Registration is by reflect.TypeOf(emptyInstance).
// Pointer fields (*T) are resolved automatically via the same mapping as T; call RegisterType once for the value type.
// Call RegisterType at application startup before the first NewTool or NewExtractor.
func RegisterType(emptyInstance any, jsonType, format string) {
	if emptyInstance == nil {
		panic("toolsy: RegisterType emptyInstance must not be nil")
	}
	if jsonType == "" {
		panic("toolsy: RegisterType jsonType must not be empty")
	}
	t := reflect.TypeOf(emptyInstance)
	s := &invopop.Schema{Type: jsonType, Format: format}
	customTypesMu.Lock()
	defer customTypesMu.Unlock()
	if customTypes == nil {
		customTypes = make(map[reflect.Type]*invopop.Schema)
	}
	customTypes[t] = s
}

// customTypeMapper returns a copy of the registered schema for t, or nil if not registered.
// It checks exact type first, then pointer element type (t.Elem()) when t is a pointer.
func customTypeMapper(t reflect.Type) *invopop.Schema {
	customTypesMu.RLock()
	defer customTypesMu.RUnlock()
	var out *invopop.Schema
	if customTypes != nil {
		if s := customTypes[t]; s != nil {
			out = s
		} else if t.Kind() == reflect.Pointer {
			if s := customTypes[t.Elem()]; s != nil {
				out = s
			}
		}
	}
	if out == nil {
		return nil
	}
	// Return a copy so callers do not share the same pointer.
	return &invopop.Schema{Type: out.Type, Format: out.Format}
}

// generateSchema produces a JSON Schema map and a compiled validator for type T.
// It is called once when building a Tool. strict sets additionalProperties: false
// for all objects (OpenAI Structured Outputs).
func generateSchema[T any](strict bool) (map[string]any, *jsonschema.Schema, error) {
	r := &invopop.Reflector{
		DoNotReference: true,
		Mapper:         customTypeMapper,
	}
	s := r.Reflect(new(T))
	if s == nil {
		return nil, nil, errNilSchema
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, nil, err
	}
	if strict {
		applyStrictMode(m)
	}
	// Remove $id/id so compiler uses our resource URL and does not resolve external refs.
	stripSchemaIDs(m)
	compiled, err := compileRawSchema(m)
	if err != nil {
		return nil, nil, err
	}
	return m, compiled, nil
}

// walkSchema recursively visits every map node in the schema tree (including $defs and definitions).
func walkSchema(m map[string]any, visit func(map[string]any)) {
	if m == nil {
		return
	}
	visit(m)
	for _, v := range m {
		switch val := v.(type) {
		case map[string]any:
			walkSchema(val, visit)
		case []any:
			for _, item := range val {
				if m2, ok := item.(map[string]any); ok {
					walkSchema(m2, visit)
				}
			}
		}
	}
}

// applyStrictMode sets additionalProperties: false for every object in the schema.
func applyStrictMode(m map[string]any) {
	walkSchema(m, func(n map[string]any) {
		if _, isObj := n["properties"]; isObj {
			n["additionalProperties"] = false
			if props, ok := n["properties"].(map[string]any); ok {
				keys := make([]string, 0, len(props))
				for k := range props {
					keys = append(keys, k)
				}
				slices.Sort(keys)
				required := make([]any, len(keys))
				for i, k := range keys {
					required[i] = k
				}
				if len(required) > 0 {
					n["required"] = required
				}
			}
		}
	})
}

var errNilSchema = errors.New("schema reflection returned nil")

// schemaURL is used when compiling so the compiler does not treat the schema as a file path.
const schemaURL = "https://toolsy.local/schema.json"

// compileRawSchema compiles a raw JSON Schema map into a validator. The map is not mutated.
// Callers must ensure the schema is valid (e.g. no conflicting $id that would break resolution).
func compileRawSchema(schemaMap map[string]any) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, schemaMap); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaURL)
}

// stripSchemaIDs removes id and $id from schema so the compiler uses the resource URL.
func stripSchemaIDs(m map[string]any) {
	walkSchema(m, func(n map[string]any) {
		delete(n, "id")
		delete(n, "$id")
	})
}
