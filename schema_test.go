package toolsy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSchema_Simple(t *testing.T) {
	type Simple struct {
		Location string `json:"location" jsonschema:"required,description=City"`
		Unit     string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit,default=celsius"`
	}
	m, compiled, err := generateSchema[Simple](false)
	require.NoError(t, err)
	require.NotNil(t, compiled)
	require.NotNil(t, m)
	// Top-level or $defs: get the object that has "properties" (may be root or a $defs entry)
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
