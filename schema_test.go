package toolsy

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findSchemaObject returns the first map in schemaMap that has "properties" (root or inside $defs).
// Used by tests to assert on additionalProperties, required, etc.
func findSchemaObject(schemaMap map[string]any) map[string]any {
	if schemaMap == nil {
		return nil
	}
	if schemaMap["properties"] != nil {
		return schemaMap
	}
	if defs, ok := schemaMap["$defs"].(map[string]any); ok {
		for _, v := range defs {
			if o, ok := v.(map[string]any); ok && o["properties"] != nil {
				return o
			}
		}
	}
	return nil
}

// snapshotAndRestoreCustomTypes backs up the global custom type registry and registers t.Cleanup
// to restore it. Use in tests that call RegisterType so they do not affect other tests.
// Do not run such tests with t.Parallel().
func snapshotAndRestoreCustomTypes(t *testing.T) {
	t.Helper()
	customTypesMu.Lock()
	before := make(map[reflect.Type]*jsonschema.Schema)
	maps.Copy(before, customTypes)
	customTypesMu.Unlock()
	t.Cleanup(func() {
		customTypesMu.Lock()
		customTypes = before
		customTypesMu.Unlock()
	})
}

func TestGenerateSchema_Simple(t *testing.T) {
	type Simple struct {
		Location string `json:"location" jsonschema:"City name"`
		Unit     string `json:"unit,omitempty" jsonschema:"Temperature unit"`
	}
	m, resolved, err := generateSchema[Simple](false)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, m)
	obj := findSchemaObject(m)
	require.NotNil(t, obj, "expected root or $defs with properties")
	props, ok := obj["properties"].(map[string]any)
	require.True(t, ok, "expected properties map")
	assert.Contains(t, props, "location")
	assert.Contains(t, props, "unit")
}

func TestGenerateSchema_StrictMode(t *testing.T) {
	type Nested struct {
		A string `json:"a"`
	}
	type Root struct {
		X string `json:"x"`
		N Nested `json:"n"`
	}
	m, _, err := generateSchema[Root](true)
	require.NoError(t, err)
	require.NotNil(t, m)
	// All objects should have additionalProperties: false
	var check func(map[string]any)
	check = func(m map[string]any) {
		if m == nil {
			return
		}
		if _, hasProps := m["properties"]; hasProps {
			v, ok := m["additionalProperties"]
			assert.True(t, ok, "expected additionalProperties in object schema")
			assert.Equal(t, false, v)
		}
		for _, val := range m {
			switch v := val.(type) {
			case map[string]any:
				check(v)
			case []any:
				for _, item := range v {
					if m2, ok := item.(map[string]any); ok {
						check(m2)
					}
				}
			}
		}
		if defs, ok := m["$defs"].(map[string]any); ok {
			for _, d := range defs {
				if m2, ok := d.(map[string]any); ok {
					check(m2)
				}
			}
		}
	}
	check(m)
}

func TestApplyStrictMode(t *testing.T) {
	m := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "string"},
			"b": map[string]any{
				"type":       "object",
				"properties": map[string]any{"c": map[string]any{"type": "integer"}},
			},
		},
	}
	applyStrictMode(m)
	assert.Equal(t, false, m["additionalProperties"])
	props := m["properties"].(map[string]any)
	assert.Equal(t, false, props["b"].(map[string]any)["additionalProperties"])
	required := m["required"].([]any)
	assert.Len(t, required, 2)
}

func TestGenerateSchema_CompiledValidates(t *testing.T) {
	type Args struct {
		X int `json:"x"`
	}
	_, resolved, err := generateSchema[Args](false)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// Valid JSON matching schema
	var parsed any
	require.NoError(t, json.Unmarshal([]byte(`{"x": 1}`), &parsed))
	err = resolved.Validate(parsed)
	assert.NoError(t, err)
	// Invalid: wrong type
	var parsedBad any
	require.NoError(t, json.Unmarshal([]byte(`{"x": "not a number"}`), &parsedBad))
	err = resolved.Validate(parsedBad)
	assert.Error(t, err)
}

func FuzzValidate(f *testing.F) {
	type Args struct {
		X int `json:"x"`
	}
	_, resolved, err := generateSchema[Args](false)
	if err != nil {
		f.Skip("generateSchema failed")
	}
	f.Add([]byte(`{"x": 1}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"x": "y"}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		var instance any
		_ = json.Unmarshal(data, &instance)
		_ = resolved.Validate(instance)
	})
}

func TestRegisterType_ValueType(t *testing.T) {
	snapshotAndRestoreCustomTypes(t)
	type MyMoney struct{}
	RegisterType(MyMoney{}, "number", "decimal")
	type Args struct {
		Amount MyMoney `json:"amount"`
	}
	m, _, err := generateSchema[Args](false)
	require.NoError(t, err)
	require.NotNil(t, m)
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	amount, ok := props["amount"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "number", amount["type"])
	assert.Equal(t, "decimal", amount["format"])
}

func TestRegisterType_PointerFieldUsesValueMapping(t *testing.T) {
	snapshotAndRestoreCustomTypes(t)
	type MyMoney struct{}
	RegisterType(MyMoney{}, "number", "decimal")
	type Args struct {
		Amount *MyMoney `json:"amount,omitempty"`
	}
	m, _, err := generateSchema[Args](false)
	require.NoError(t, err)
	require.NotNil(t, m)
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	amount, ok := props["amount"].(map[string]any)
	require.True(t, ok)
	// google/jsonschema-go may output "type": "number", "types": ["null", "number"], or "type": ["null", "number"] for pointer fields
	hasNumber := false
	if typ, ok := amount["type"].(string); ok {
		hasNumber = typ == "number"
	} else if types, ok := amount["types"].([]any); ok {
		hasNumber = slices.Contains(types, "number")
	} else if typeArr, ok := amount["type"].([]any); ok {
		hasNumber = slices.Contains(typeArr, "number")
	}
	assert.True(t, hasNumber, "amount schema must allow number (type or types): %v", amount)
	assert.Equal(t, "decimal", amount["format"])
}

// noRefInSchemaTree returns false if any node in schemaMap has a "$ref" key (LLM inline requirement).
func noRefInSchemaTree(schemaMap map[string]any) bool {
	if schemaMap == nil {
		return true
	}
	if _, has := schemaMap["$ref"]; has {
		return false
	}
	for _, val := range schemaMap {
		switch v := val.(type) {
		case map[string]any:
			if !noRefInSchemaTree(v) {
				return false
			}
		case []any:
			for _, item := range v {
				if m2, ok := item.(map[string]any); ok {
					if !noRefInSchemaTree(m2) {
						return false
					}
				}
			}
		}
	}
	return true
}

func TestGenerateSchema_NoTopLevelRefOrDefs(t *testing.T) {
	type Nested struct {
		A string `json:"a"`
	}
	type Root struct {
		N Nested `json:"n"`
	}
	schemaMap, _, err := generateSchema[Root](false)
	require.NoError(t, err)
	require.NotNil(t, schemaMap)
	assert.Nil(t, schemaMap["$ref"], "root schema must not contain $ref for LLM compatibility")
	assert.Nil(t, schemaMap["$defs"], "root schema must not contain $defs for LLM compatibility")
	assert.True(t, noRefInSchemaTree(schemaMap), "schema tree must not contain $ref in any node")
}

func TestRegisterType_InvalidArgs_Panic(t *testing.T) {
	snapshotAndRestoreCustomTypes(t)
	assert.Panics(t, func() { RegisterType(nil, "string", "uuid") })
	assert.Panics(t, func() { RegisterType(struct{}{}, "", "uuid") })
}
