package toolsy

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSchemaConfig(strict bool) SchemaConfig {
	return SchemaConfig{Strict: strict}
}

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

func TestGenerateSchema_Simple(t *testing.T) {
	type Simple struct {
		Location string `json:"location"       jsonschema:"City name"`
		Unit     string `json:"unit,omitempty" jsonschema:"Temperature unit"`
	}
	m, resolved, err := generateSchema[Simple](testSchemaConfig(false))
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

func TestGenerateSchema_StructTagsDescriptionAndEnum(t *testing.T) {
	type WithTags struct {
		Status string `description:"System status" enum:"ok,fail" json:"status"`
	}
	m, _, err := generateSchema[WithTags](testSchemaConfig(false))
	require.NoError(t, err)
	require.NotNil(t, m)
	obj := findSchemaObject(m)
	require.NotNil(t, obj)
	props, ok := obj["properties"].(map[string]any)
	require.True(t, ok)
	statusProp, ok := props["status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "System status", statusProp["description"])
	enumArr, ok := statusProp["enum"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"ok", "fail"}, enumArr)
}

func TestGenerateSchema_StrictMode(t *testing.T) {
	type Nested struct {
		A string `json:"a"`
	}
	type Root struct {
		X string `json:"x"`
		N Nested `json:"n"`
	}
	m, _, err := generateSchema[Root](testSchemaConfig(true))
	require.NoError(t, err)
	require.NotNil(t, m)
	assertStrictModeAdditionalProperties(t, m)
}

func assertStrictModeAdditionalProperties(t *testing.T, root map[string]any) {
	t.Helper()
	var check func(map[string]any)
	check = func(m map[string]any) {
		if m == nil {
			return
		}
		assertObjectHasAdditionalPropertiesFalse(t, m)
		for _, val := range m {
			strictWalkSchemaChildren(t, check, val)
		}
		if defs, ok := m["$defs"].(map[string]any); ok {
			for _, d := range defs {
				if m2, ok := d.(map[string]any); ok {
					check(m2)
				}
			}
		}
	}
	check(root)
}

func assertObjectHasAdditionalPropertiesFalse(t *testing.T, m map[string]any) {
	t.Helper()
	if _, hasProps := m["properties"]; !hasProps {
		return
	}
	v, ok := m["additionalProperties"]
	assert.True(t, ok, "expected additionalProperties in object schema")
	assert.Equal(t, false, v)
}

func strictWalkSchemaChildren(t *testing.T, check func(map[string]any), val any) {
	t.Helper()
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
	_, resolved, err := generateSchema[Args](testSchemaConfig(false))
	require.NoError(t, err)
	require.NotNil(t, resolved)
	var parsed any
	require.NoError(t, json.Unmarshal([]byte(`{"x": 1}`), &parsed))
	err = resolved.Validate(parsed)
	assert.NoError(t, err)
	var parsedBad any
	require.NoError(t, json.Unmarshal([]byte(`{"x": "not a number"}`), &parsedBad))
	err = resolved.Validate(parsedBad)
	assert.Error(t, err)
}

func FuzzValidate(f *testing.F) {
	type Args struct {
		X int `json:"x"`
	}
	_, resolved, err := generateSchema[Args](testSchemaConfig(false))
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

func TestSchemaRegistryRegisterType_ValueType(t *testing.T) {
	registry := NewSchemaRegistry()
	type MyMoney struct{}
	registry.RegisterType(MyMoney{}, "number", "decimal")
	type Args struct {
		Amount MyMoney `json:"amount"`
	}
	m, _, err := generateSchema[Args](SchemaConfig{Registry: registry})
	require.NoError(t, err)
	require.NotNil(t, m)
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	amount, ok := props["amount"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "number", amount["type"])
	assert.Equal(t, "decimal", amount["format"])
}

func TestSchemaRegistryRegisterType_PointerFieldUsesValueMapping(t *testing.T) {
	registry := NewSchemaRegistry()
	type MyMoney struct{}
	registry.RegisterType(MyMoney{}, "number", "decimal")
	type Args struct {
		Amount *MyMoney `json:"amount,omitempty"`
	}
	m, _, err := generateSchema[Args](SchemaConfig{Registry: registry})
	require.NoError(t, err)
	require.NotNil(t, m)
	props, ok := m["properties"].(map[string]any)
	require.True(t, ok)
	amount, ok := props["amount"].(map[string]any)
	require.True(t, ok)
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

func TestSchemaRegistry_IsolatedByDefault(t *testing.T) {
	type MyMoney struct{}
	type Args struct {
		Amount MyMoney `json:"amount"`
	}
	registry := NewSchemaRegistry()
	registry.RegisterType(MyMoney{}, "number", "decimal")

	toolWithShared, err := NewTool(
		"shared",
		"desc",
		func(_ context.Context, _ RunContext, _ Args) (struct{}, error) { return struct{}{}, nil },
		WithSchemaRegistry(registry),
	)
	require.NoError(t, err)
	toolWithLocal, err := NewTool(
		"local",
		"desc",
		func(_ context.Context, _ RunContext, _ Args) (struct{}, error) { return struct{}{}, nil },
	)
	require.NoError(t, err)

	sharedProps := toolWithShared.Manifest().Parameters["properties"].(map[string]any)
	localProps := toolWithLocal.Manifest().Parameters["properties"].(map[string]any)
	assert.Equal(t, "decimal", sharedProps["amount"].(map[string]any)["format"])
	assert.NotEqual(t, "decimal", localProps["amount"].(map[string]any)["format"])
}

func TestNewExtractorWithConfig_UsesSharedRegistry(t *testing.T) {
	registry := NewSchemaRegistry()
	type MyMoney struct{}
	registry.RegisterType(MyMoney{}, "number", "decimal")
	type Args struct {
		Amount MyMoney `json:"amount"`
	}
	extractor, err := NewExtractorWithConfig[Args](SchemaConfig{Registry: registry})
	require.NoError(t, err)
	props := extractor.Schema()["properties"].(map[string]any)
	assert.Equal(t, "decimal", props["amount"].(map[string]any)["format"])
}

func noRefInSchemaTree(schemaMap map[string]any) bool {
	if schemaMap == nil {
		return true
	}
	if _, has := schemaMap["$ref"]; has {
		return false
	}
	for _, val := range schemaMap {
		if !noRefInSchemaValue(val) {
			return false
		}
	}
	return true
}

func noRefInSchemaValue(val any) bool {
	switch v := val.(type) {
	case map[string]any:
		return noRefInSchemaTree(v)
	case []any:
		for _, item := range v {
			if m2, ok := item.(map[string]any); ok {
				if !noRefInSchemaTree(m2) {
					return false
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
	schemaMap, _, err := generateSchema[Root](testSchemaConfig(false))
	require.NoError(t, err)
	require.NotNil(t, schemaMap)
	assert.Nil(t, schemaMap["$ref"], "root schema must not contain $ref for LLM compatibility")
	assert.Nil(t, schemaMap["$defs"], "root schema must not contain $defs for LLM compatibility")
	assert.True(t, noRefInSchemaTree(schemaMap), "schema tree must not contain $ref in any node")
}

func TestSchemaRegistryRegisterType_InvalidArgs_Panic(t *testing.T) {
	registry := NewSchemaRegistry()
	assert.Panics(t, func() { registry.RegisterType(nil, "string", "uuid") })
	assert.Panics(t, func() { registry.RegisterType(struct{}{}, "", "uuid") })
}
