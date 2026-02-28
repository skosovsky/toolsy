package toolsy

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

var (
	customTypesMu sync.RWMutex
	customTypes   = make(map[reflect.Type]*jsonschema.Schema)
)

// RegisterType registers a custom Go type to be mapped to a JSON Schema type/format in generated schemas.
// emptyInstance is a value of the type to register (e.g. uuid.UUID{}, or MyMoney{}); it must not be nil.
// jsonType is the JSON Schema type (e.g. "string", "number"); it must not be empty.
// format is optional (e.g. "uuid", "decimal"). Registration is by reflect.TypeOf(emptyInstance).
// Pointer fields (*T) use the same mapping as T; call RegisterType once for the value type.
// Call RegisterType at application startup before the first NewTool or NewExtractor.
func RegisterType(emptyInstance any, jsonType, format string) {
	if emptyInstance == nil {
		panic("toolsy: RegisterType emptyInstance must not be nil")
	}
	if jsonType == "" {
		panic("toolsy: RegisterType jsonType must not be empty")
	}
	t := reflect.TypeOf(emptyInstance)
	s := &jsonschema.Schema{Type: jsonType, Format: format}
	customTypesMu.Lock()
	defer customTypesMu.Unlock()
	if customTypes == nil {
		customTypes = make(map[reflect.Type]*jsonschema.Schema)
	}
	customTypes[t] = s
}

// buildTypeSchemas returns a copy of registered type schemas for use in ForOptions.
// Caller holds no lock; safe for concurrent use with RegisterType.
func buildTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	customTypesMu.RLock()
	defer customTypesMu.RUnlock()
	out := make(map[reflect.Type]*jsonschema.Schema, len(customTypes))
	for t, s := range customTypes {
		if s != nil {
			out[t] = s.CloneSchemas()
		}
	}
	return out
}

// generateSchema produces a JSON Schema map and a resolved validator for type T.
// It is called once when building a Tool. strict sets additionalProperties: false
// for all objects (OpenAI Structured Outputs).
func generateSchema[T any](strict bool) (map[string]any, *jsonschema.Resolved, error) {
	opts := &jsonschema.ForOptions{TypeSchemas: buildTypeSchemas()}
	schema, err := jsonschema.For[T](opts)
	if err != nil {
		return nil, nil, err
	}
	if schema == nil {
		return nil, nil, errNilSchema
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, nil, err
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return nil, nil, err
	}
	enrichSchemaFromStructTags(schemaMap, reflect.TypeOf(*new(T)))
	if strict {
		applyStrictMode(schemaMap)
	}
	stripSchemaIDs(schemaMap)
	resolved, err := compileRawSchema(schemaMap)
	if err != nil {
		return nil, nil, err
	}
	return schemaMap, resolved, nil
}

// enrichSchemaFromStructTags adds description and enum from struct tags to root-level properties.
// typ may be a pointer; json tag (first part before comma) is used to match property keys.
func enrichSchemaFromStructTags(schemaMap map[string]any, typ reflect.Type) {
	if schemaMap == nil || typ == nil {
		return
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	props, ok := schemaMap["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return
	}
	// Build json name -> field for root struct
	jsonToField := make(map[string]reflect.StructField)
	for field := range typ.Fields() {
		field := field
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		jsonToField[jsonTag] = field
	}
	for key, val := range props {
		prop, ok := val.(map[string]any)
		if !ok {
			continue
		}
		field, ok := jsonToField[key]
		if !ok {
			continue
		}
		if desc := field.Tag.Get("description"); desc != "" {
			prop["description"] = desc
		}
		if enumStr := field.Tag.Get("enum"); enumStr != "" {
			parts := strings.Split(enumStr, ",")
			enum := make([]any, len(parts))
			for i, p := range parts {
				enum[i] = strings.TrimSpace(p)
			}
			prop["enum"] = enum
		}
	}
}

// walkSchema recursively visits every map node in the schema tree (including $defs and definitions).
func walkSchema(schemaMap map[string]any, visit func(map[string]any)) {
	if schemaMap == nil {
		return
	}
	visit(schemaMap)
	for _, val := range schemaMap {
		switch v := val.(type) {
		case map[string]any:
			walkSchema(v, visit)
		case []any:
			for _, item := range v {
				if m2, ok := item.(map[string]any); ok {
					walkSchema(m2, visit)
				}
			}
		}
	}
}

// applyStrictMode sets additionalProperties: false for every object in the schema.
func applyStrictMode(schemaMap map[string]any) {
	walkSchema(schemaMap, func(n map[string]any) {
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

// compileRawSchema compiles a raw JSON Schema map into a resolved validator. The map is not mutated.
// Callers must ensure the schema is valid (e.g. no conflicting $id that would break resolution).
func compileRawSchema(schemaMap map[string]any) (*jsonschema.Resolved, error) {
	data, err := json.Marshal(schemaMap)
	if err != nil {
		return nil, err
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Resolve(nil)
}

// stripSchemaIDs removes id and $id from schema so resolution does not depend on them.
func stripSchemaIDs(schemaMap map[string]any) {
	walkSchema(schemaMap, func(n map[string]any) {
		delete(n, "id")
		delete(n, "$id")
	})
}
