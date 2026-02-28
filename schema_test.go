package toolsy

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	invopop "github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotAndRestoreCustomTypes backs up the global custom type registry and registers t.Cleanup
// to restore it. Use in tests that call RegisterType so they do not affect other tests.
// Do not run such tests with t.Parallel().
func snapshotAndRestoreCustomTypes(t *testing.T) {
	t.Helper()
	customTypesMu.Lock()
	before := make(map[reflect.Type]*invopop.Schema)
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
		Location string `json:"location" jsonschema:"required,description=City"`
		Unit     string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit,default=celsius"`
	}
	m, compiled, err := generateSchema[Simple](false)
	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.NotNil(t, m)
	// Root or $defs: get the object that has "properties" (DoNotReference inlines; we support both)
	var obj map[string]any
	if m["properties"] != nil {
		obj = m
	} else if defs, ok := m["$defs"].(map[string]any); ok {
		for _, v := range defs {
			if o, ok := v.(map[string]any); ok {
				obj = o
				break
			}
		}
	}
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
	_, compiled, err := generateSchema[Args](false)
	require.NoError(t, err)
	require.NotNil(t, compiled)
	// Valid JSON matching schema
	var v any
	require.NoError(t, json.Unmarshal([]byte(`{"x": 1}`), &v))
	err = compiled.Validate(v)
	assert.NoError(t, err)
	// Invalid: wrong type
	var vBad any
	require.NoError(t, json.Unmarshal([]byte(`{"x": "not a number"}`), &vBad))
	err = compiled.Validate(vBad)
	assert.Error(t, err)
}

func FuzzValidate(f *testing.F) {
	type Args struct {
		X int `json:"x"`
	}
	_, compiled, err := generateSchema[Args](false)
	if err != nil {
		f.Skip("generateSchema failed")
	}
	f.Add([]byte(`{"x": 1}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"x": "y"}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		var v any
		_ = json.Unmarshal(data, &v)
		_ = compiled.Validate(v)
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
	assert.Equal(t, "number", amount["type"])
	assert.Equal(t, "decimal", amount["format"])
}

func TestGenerateSchema_NoRefsOrDefs(t *testing.T) {
	type Nested struct {
		A string `json:"a"`
	}
	type Root struct {
		N Nested `json:"n"`
	}
	m, _, err := generateSchema[Root](false)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Nil(t, m["$ref"], "schema must not contain $ref")
	assert.Nil(t, m["$defs"], "schema must not contain $defs with DoNotReference")
}

func TestRegisterType_InvalidArgs_Panic(t *testing.T) {
	snapshotAndRestoreCustomTypes(t)
	assert.Panics(t, func() { RegisterType(nil, "string", "uuid") })
	assert.Panics(t, func() { RegisterType(struct{}{}, "", "uuid") })
}
