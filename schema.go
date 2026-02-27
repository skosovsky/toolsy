package toolsy

import (
	"encoding/json"
	"errors"
	"slices"

	invopop "github.com/invopop/jsonschema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// generateSchema produces a JSON Schema map and a compiled validator for type T.
// It is called once when building a Tool. strict sets additionalProperties: false
// for all objects (OpenAI Structured Outputs).
func generateSchema[T any](strict bool) (map[string]any, *jsonschema.Schema, error) {
	r := &invopop.Reflector{}
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
	// Use an absolute URL so the compiler never treats it as a file path (avoids failures
	// when cwd contains spaces or non-ASCII characters).
	schemaURL := "https://toolsy.local/schema.json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, m); err != nil {
		return nil, nil, err
	}
	compiled, err := compiler.Compile(schemaURL)
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

// stripSchemaIDs removes id and $id from schema so the compiler uses the resource URL.
func stripSchemaIDs(m map[string]any) {
	walkSchema(m, func(n map[string]any) {
		delete(n, "id")
		delete(n, "$id")
	})
}
