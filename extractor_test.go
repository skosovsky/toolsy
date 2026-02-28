package toolsy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExtractor_Success(t *testing.T) {
	t.Parallel()
	type Args struct {
		X int `json:"x"`
	}
	ext, err := NewExtractor[Args](false)
	require.NoError(t, err)
	require.NotNil(t, ext)
	schema := ext.Schema()
	require.NotNil(t, schema)
}

func TestNewExtractor_Strict(t *testing.T) {
	t.Parallel()
	type Args struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	ext, err := NewExtractor[Args](true)
	require.NoError(t, err)
	require.NotNil(t, ext)
	schema := ext.Schema()
	require.NotNil(t, schema)
	// Find the object node (root or $defs entry; DoNotReference inlines, but we support both shapes)
	var obj map[string]any
	if schema["properties"] != nil {
		obj = schema
	} else if defs, ok := schema["$defs"].(map[string]any); ok {
		for _, v := range defs {
			if o, ok := v.(map[string]any); ok && o["properties"] != nil {
				obj = o
				break
			}
		}
	}
	require.NotNil(t, obj, "expected object with properties in schema")
	assert.Equal(t, false, obj["additionalProperties"])
	// Strict mode also makes all properties required
	required, ok := obj["required"].([]any)
	require.True(t, ok, "strict schema must have required array")
	require.Len(t, required, 2, "required must list all properties (a, b)")
	// Order is deterministic (slices.Sort in applyStrictMode)
	assert.Equal(t, "a", required[0])
	assert.Equal(t, "b", required[1])
}

func TestExtractor_ParseAndValidate_Success(t *testing.T) {
	t.Parallel()
	type Args struct {
		X int    `json:"x"`
		S string `json:"s"`
	}
	ext, err := NewExtractor[Args](false)
	require.NoError(t, err)
	args, err := ext.ParseAndValidate([]byte(`{"x": 42, "s": "hello"}`))
	require.NoError(t, err)
	assert.Equal(t, 42, args.X)
	assert.Equal(t, "hello", args.S)
}

func TestExtractor_ParseAndValidate_InvalidJSON(t *testing.T) {
	t.Parallel()
	type Args struct {
		X int `json:"x"`
	}
	ext, err := NewExtractor[Args](false)
	require.NoError(t, err)
	_, err = ext.ParseAndValidate([]byte(`{invalid`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestExtractor_ParseAndValidate_SchemaViolation(t *testing.T) {
	t.Parallel()
	type Args struct {
		Unit string `json:"unit" jsonschema:"enum=celsius|fahrenheit"`
	}
	ext, err := NewExtractor[Args](false)
	require.NoError(t, err)
	_, err = ext.ParseAndValidate([]byte(`{"unit": "kelvin"}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestExtractor_ParseAndValidate_Validatable(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor[validatableArgs](false)
	require.NoError(t, err)
	// Valid: low <= high
	args, err := ext.ParseAndValidate([]byte(`{"low": 1, "high": 10}`))
	require.NoError(t, err)
	assert.Equal(t, 1, args.Low)
	assert.Equal(t, 10, args.High)
	// Invalid: low > high
	_, err = ext.ParseAndValidate([]byte(`{"low": 10, "high": 5}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	assert.ErrorIs(t, err, ErrValidation)
}

func TestExtractor_ParseAndValidate_ValidatablePointer(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor[pointerValidatableArgs](false)
	require.NoError(t, err)
	// Valid: min <= max
	args, err := ext.ParseAndValidate([]byte(`{"min": 1, "max": 10}`))
	require.NoError(t, err)
	assert.Equal(t, 1, args.Min)
	assert.Equal(t, 10, args.Max)
	// Invalid: min > max — pointer receiver Validate() is called
	_, err = ext.ParseAndValidate([]byte(`{"min": 10, "max": 5}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	assert.ErrorIs(t, err, ErrValidation)
}

// TestExtractor_ParseAndValidate_PointerT ensures Extractor[*T] runs Validatable when T is pointer type.
func TestExtractor_ParseAndValidate_PointerT(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor[*pointerValidatableArgs](false)
	require.NoError(t, err)
	// Valid: min <= max
	args, err := ext.ParseAndValidate([]byte(`{"min": 1, "max": 10}`))
	require.NoError(t, err)
	require.NotNil(t, args)
	assert.Equal(t, 1, args.Min)
	assert.Equal(t, 10, args.Max)
	// Invalid: min > max — Validate() on *pointerValidatableArgs is called
	_, err = ext.ParseAndValidate([]byte(`{"min": 10, "max": 5}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	assert.ErrorIs(t, err, ErrValidation)
}

func TestExtractor_Schema_ReturnsCopy(t *testing.T) {
	t.Parallel()
	type Args struct {
		X int `json:"x"`
	}
	ext, err := NewExtractor[Args](false)
	require.NoError(t, err)
	s1 := ext.Schema()
	require.NotNil(t, s1)
	s1["mutated"] = true
	s2 := ext.Schema()
	_, ok := s2["mutated"]
	assert.False(t, ok, "mutating returned map must not affect subsequent Schema()")
}

// TestExtractor_ParseAndValidate_StrictMissingRequired checks strict mode rejects missing required field.
func TestExtractor_ParseAndValidate_StrictMissingRequired(t *testing.T) {
	t.Parallel()
	type Args struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	ext, err := NewExtractor[Args](true)
	require.NoError(t, err)
	_, err = ext.ParseAndValidate([]byte(`{"a": "only"}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

// clientErrValidatable returns ClientError from Validate for passthrough test.
type clientErrValidatable struct {
	V int `json:"v"`
}

func (c clientErrValidatable) Validate() error {
	if c.V < 0 {
		return &ClientError{Reason: "v must be >= 0", Err: ErrValidation}
	}
	return nil
}

func TestExtractor_ParseAndValidate_ValidatableClientErrorPassthrough(t *testing.T) {
	t.Parallel()
	ext, err := NewExtractor[clientErrValidatable](false)
	require.NoError(t, err)
	_, err = ext.ParseAndValidate([]byte(`{"v": -1}`))
	require.Error(t, err)
	assert.True(t, IsClientError(err))
	var ce *ClientError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "v must be >= 0", ce.Reason)
}

// countValidatable counts Validate() calls for double-invocation test.
type countValidatable struct {
	X int `json:"x"`
}

var layer2ValidateCallCount int

func (c countValidatable) Validate() error {
	layer2ValidateCallCount++
	return nil
}

// TestExtractor_ParseAndValidate_ValidatableNotCalledTwice ensures Layer-2 validation
// runs at most once per parse (no double call for pointer-receiver fallback).
func TestExtractor_ParseAndValidate_ValidatableNotCalledTwice(t *testing.T) {
	layer2ValidateCallCount = 0
	defer func() { layer2ValidateCallCount = 0 }()
	ext, err := NewExtractor[countValidatable](false)
	require.NoError(t, err)
	_, err = ext.ParseAndValidate([]byte(`{"x": 1}`))
	require.NoError(t, err)
	assert.Equal(t, 1, layer2ValidateCallCount, "Validate() must be called exactly once")
}

// TestExtractor_ParseAndValidate_InterfaceT_Null_NoPanic ensures ParseAndValidate with T=any
// and JSON "null" does not panic (runLayer2Validation guards reflect.TypeOf(nil)).
func TestExtractor_ParseAndValidate_InterfaceT_Null_NoPanic(t *testing.T) {
	ext, err := NewExtractor[any](false)
	if err != nil {
		t.Skip("NewExtractor[any] not supported by schema generator")
	}
	// Must not panic; result may be nil or schema may reject null
	_, _ = ext.ParseAndValidate([]byte("null"))
}

// TestExtractor_ParseAndValidate_InterfaceT_Object_NoPanic ensures ParseAndValidate with T=any
// and JSON object does not panic.
func TestExtractor_ParseAndValidate_InterfaceT_Object_NoPanic(t *testing.T) {
	ext, err := NewExtractor[any](false)
	if err != nil {
		t.Skip("NewExtractor[any] not supported by schema generator")
	}
	_, _ = ext.ParseAndValidate([]byte(`{}`))
}
