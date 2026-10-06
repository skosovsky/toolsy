package toolsy

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// SchemaRegistry stores custom type to JSON Schema mappings for typed builders/extractors.
type SchemaRegistry struct {
	mu    sync.RWMutex
	types map[reflect.Type]*jsonschema.Schema
}

// NewSchemaRegistry creates an empty schema registry.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		mu:    sync.RWMutex{},
		types: make(map[reflect.Type]*jsonschema.Schema),
	}
}

// RegisterType registers a custom Go type to be mapped to a JSON Schema type/format in generated schemas.
// emptyInstance is a value of the type to register (for example, a UUID or money value); it must not be nil.
// jsonType is the JSON Schema type (e.g. "string", "number"); it must not be empty.
// format is optional (e.g. "uuid", "decimal"). Registration is by [reflect.TypeOf](emptyInstance).
// Pointer fields (*T) use the same mapping as T; call RegisterType once for the value type.
func (r *SchemaRegistry) RegisterType(emptyInstance any, jsonType, format string) {
	if emptyInstance == nil {
		panic("toolsy: RegisterType emptyInstance must not be nil")
	}
	if jsonType == "" {
		panic("toolsy: RegisterType jsonType must not be empty")
	}
	t := reflect.TypeOf(emptyInstance)
	s := &jsonschema.Schema{Type: jsonType, Format: format}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.types == nil {
		r.types = make(map[reflect.Type]*jsonschema.Schema)
	}
	r.types[t] = s
}

func ensureSchemaConfig(cfg SchemaConfig) SchemaConfig {
	if cfg.Registry == nil {
		cfg.Registry = NewSchemaRegistry()
	}
	return cfg
}

func (r *SchemaRegistry) buildTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[reflect.Type]*jsonschema.Schema, len(r.types))
	for t, s := range r.types {
		if s != nil {
			out[t] = s.CloneSchemas()
		}
	}
	return out
}

// generateSchema produces a JSON Schema map and a resolved validator for type T.
// It is called once when building a Tool. cfg.Strict sets additionalProperties: false
// for all objects (OpenAI Structured Outputs). cfg.Registry controls custom type mappings.
func generateSchema[T any](cfg SchemaConfig) (map[string]any, schemaValidator, error) {
	return generateSchemaWithRawDefault[T](cfg, &jsonschema.Schema{Type: "object"})
}

func generateSchemaWithRawDefault[T any](
	cfg SchemaConfig,
	rawDefault *jsonschema.Schema,
) (map[string]any, schemaValidator, error) {
	cfg = ensureSchemaConfig(cfg)
	typeSchemas := cfg.Registry.buildTypeSchemas()
	if _, overridden := typeSchemas[reflect.TypeFor[json.RawMessage]()]; !overridden {
		typeSchemas[reflect.TypeFor[json.RawMessage]()] = rawDefault
	}
	opts := &jsonschema.ForOptions{TypeSchemas: typeSchemas}
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
	value, decodeErr := jsonschemax.Decode(data)
	if decodeErr != nil {
		return nil, nil, decodeErr
	}
	schemaMap, ok := value.(map[string]any)
	if !ok {
		return nil, nil, errors.New("generated schema must be an object")
	}
	enrichSchemaFromStructTags(schemaMap, reflect.TypeOf(*new(T)))
	if cfg.Strict {
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
	jsonToField := make(map[string]reflect.StructField)
	for field := range typ.Fields() {
		jsonTag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
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
		enrichPropertyFromStructField(prop, field)
	}
}

func enrichPropertyFromStructField(prop map[string]any, field reflect.StructField) {
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

// walkSchema visits schema objects, excluding property-name maps and literal
// enum/const/default values. Those may contain keys named id or properties.
func walkSchema(schemaMap map[string]any, visit func(map[string]any)) {
	if schemaMap == nil {
		return
	}
	visit(schemaMap)
	for key, value := range schemaMap {
		switch key {
		case "$defs", "definitions", "properties", "patternProperties", "dependentSchemas", "dependencies":
			if children, ok := value.(map[string]any); ok {
				for _, child := range children {
					walkSchemaChild(child, visit)
				}
			}
		case "allOf", "anyOf", "oneOf", "prefixItems", "items", "additionalItems", "contains",
			"additionalProperties", "unevaluatedProperties", "unevaluatedItems", "propertyNames",
			"not", "if", "then", "else", "contentSchema":
			walkSchemaChild(value, visit)
		}
	}
}

func walkSchemaChild(value any, visit func(map[string]any)) {
	switch child := value.(type) {
	case map[string]any:
		walkSchema(child, visit)
	case []any:
		for _, item := range child {
			if schema, ok := item.(map[string]any); ok {
				walkSchema(schema, visit)
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
func compileRawSchema(schemaMap map[string]any) (schemaValidator, error) {
	return jsonschemax.Compile(schemaMap)
}

// stripSchemaIDs removes id and $id from schema so resolution does not depend on them.
func stripSchemaIDs(schemaMap map[string]any) {
	walkSchema(schemaMap, func(n map[string]any) {
		delete(n, "id")
		delete(n, "$id")
	})
}
