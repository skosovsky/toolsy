package main

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type captureHandler struct {
	calls int
	input PresenceInput
}

func (h *captureHandler) Execute(_ context.Context, input PresenceInput) (string, error) {
	h.calls++
	h.input = input
	return "ok", nil
}

func baseObject(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(exampleInput), &object))
	return object
}

func executeObject(t *testing.T, h *captureHandler, object map[string]json.RawMessage) error {
	t.Helper()
	tool, err := NewPresenceTool(h)
	require.NoError(t, err)
	data, err := json.Marshal(object)
	require.NoError(t, err)
	return tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: data},
		func(toolsy.Chunk) error { return nil },
	)
}

func TestRequiredZerosOptionalOmissionAndNullableNull(t *testing.T) {
	// Arrange.
	h := &captureHandler{}
	object := baseObject(t)
	// Act.
	err := executeObject(t, h, object)
	// Assert: required scalar/slice shapes and exact numbers compile and execute.
	require.NoError(t, err)
	require.Equal(t, 1, h.calls)
	in := h.input
	require.Empty(t, in.RString)
	require.Equal(t, "9007199254740993", in.RInteger.String())
	require.NotNil(t, in.RBoolean)
	require.False(t, *in.RBoolean)
	require.NotNil(t, in.RDatetimes)
	require.Empty(t, *in.RDatetimes)
	require.Nil(t, in.ODatetimes)
	require.NotNil(t, in.RStrings)
	require.NotNil(t, in.RIntegers)
	require.NotNil(t, in.RBooleans)
	require.Empty(t, *in.RStrings)
	require.Empty(t, *in.RIntegers)
	require.Empty(t, *in.RBooleans)
	require.Equal(t, "2026-10-07T00:00:00Z", in.RDatetime)
	require.Nil(t, in.OString)
	require.Nil(t, in.OInteger)
	require.Nil(t, in.OBoolean)
	require.Nil(t, in.ODatetime)
	require.Nil(t, in.OStrings)
	require.Nil(t, in.OIntegers)
	require.Nil(t, in.OBooleans)
	for _, raw := range []json.RawMessage{in.RNullableString, in.RNullableInteger, in.RNullableBoolean, in.RNullableStrings, in.RNullableIntegers, in.RNullableBooleans} {
		require.Equal(t, "null", string(raw))
	}
	for _, raw := range []json.RawMessage{in.ONullableString, in.ONullableInteger, in.ONullableBoolean, in.ONullableStrings, in.ONullableIntegers, in.ONullableBooleans} {
		require.Nil(t, raw)
	}
}

func TestOptionalPresentEmptyAndPrimitiveItems(t *testing.T) {
	// Arrange.
	h := &captureHandler{}
	object := baseObject(t)
	extras := map[string]string{
		"o_string":   `""`,
		"o_integer":  "0",
		"o_boolean":  "false",
		"o_datetime": `"2026-10-07T00:00:00Z"`,
		"o_strings":  "[]",
		"o_integers": "[]",
		"o_booleans": "[]",
		"r_strings":  `[""]`,
		"r_integers": "[0,9007199254740993,1.0]",
		"r_booleans": "[false]",
		"extra":      "42",
	}
	for key, raw := range extras {
		object[key] = json.RawMessage(raw)
	}
	// Act.
	err := executeObject(t, h, object)
	// Assert.
	require.NoError(t, err)
	in := h.input
	require.NotNil(t, in.OString)
	require.Empty(t, *in.OString)
	require.NotNil(t, in.OInteger)
	require.Equal(t, "0", in.OInteger.String())
	require.NotNil(t, in.OBoolean)
	require.False(t, *in.OBoolean)
	require.NotNil(t, in.ODatetime)
	require.NotNil(t, in.OStrings)
	require.Empty(t, in.OStrings)
	require.NotNil(t, in.OIntegers)
	require.Empty(t, in.OIntegers)
	require.NotNil(t, in.OBooleans)
	require.Empty(t, in.OBooleans)
	require.Equal(t, []string{""}, *in.RStrings)
	require.Equal(t, []json.Number{"0", "9007199254740993", "1.0"}, *in.RIntegers)
	require.Equal(t, []bool{false}, *in.RBooleans)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(in.RawJSON, &raw))
	require.Equal(t, "42", string(raw["extra"]))
	require.Equal(t, "[]", string(raw["o_strings"]))
	// DTO serialization is not the authoritative original presence representation.
	serialized, err := json.Marshal(in)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(serialized, &raw))
	var fresh map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(serialized, &fresh))
	require.NotContains(t, fresh, "o_strings")
	require.Equal(t, "null", string(fresh["o_nullable_string"]))
}

func TestNullableConcreteValuesAndOptionalNull(t *testing.T) {
	for _, nulls := range []bool{false, true} {
		t.Run(map[bool]string{false: "values", true: "nulls"}[nulls], func(t *testing.T) {
			// Arrange.
			h := &captureHandler{}
			object := baseObject(t)
			values := map[string]string{
				"string":   `""`,
				"integer":  "9007199254740993",
				"boolean":  "false",
				"strings":  "[]",
				"integers": "[1.0]",
				"booleans": "[false]",
			}
			for name, raw := range values {
				if nulls {
					raw = "null"
				}
				for _, prefix := range []string{"r", "o"} {
					object[prefix+"_nullable_"+name] = json.RawMessage(raw)
				}
			}
			// Act.
			err := executeObject(t, h, object)
			// Assert: exact raw values survive for required and optional unions.
			require.NoError(t, err)
			actual := map[string]json.RawMessage{
				"r_nullable_string":   h.input.RNullableString,
				"o_nullable_string":   h.input.ONullableString,
				"r_nullable_integer":  h.input.RNullableInteger,
				"o_nullable_integer":  h.input.ONullableInteger,
				"r_nullable_boolean":  h.input.RNullableBoolean,
				"o_nullable_boolean":  h.input.ONullableBoolean,
				"r_nullable_strings":  h.input.RNullableStrings,
				"o_nullable_strings":  h.input.ONullableStrings,
				"r_nullable_integers": h.input.RNullableIntegers,
				"o_nullable_integers": h.input.ONullableIntegers,
				"r_nullable_booleans": h.input.RNullableBooleans,
				"o_nullable_booleans": h.input.ONullableBooleans,
			}
			for key, value := range actual {
				require.Equal(t, string(object[key]), string(value), key)
			}
		})
	}
}

func TestInvalidPresenceFailsBeforeDispatch(t *testing.T) {
	// Arrange: derive required and nonnullable fields from the example source contract.
	data, err := os.ReadFile("presence.json")
	require.NoError(t, err)
	var manifest struct {
		Parameters struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type json.RawMessage `json:"type"`
			} `json:"properties"`
		} `json:"parameters"`
	}
	require.NoError(t, json.Unmarshal(data, &manifest))
	base := baseObject(t)
	for _, key := range manifest.Parameters.Required {
		t.Run("omit "+key, func(t *testing.T) {
			h := &captureHandler{}
			object := maps.Clone(base)
			delete(object, key)
			// Act.
			err := executeObject(t, h, object)
			// Assert.
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Zero(t, h.calls)
		})
	}
	for key, property := range manifest.Parameters.Properties {
		if len(property.Type) > 0 && property.Type[0] == '[' {
			continue
		}
		t.Run("null "+key, func(t *testing.T) {
			h := &captureHandler{}
			object := maps.Clone(base)
			object[key] = json.RawMessage("null")
			// Act.
			err := executeObject(t, h, object)
			// Assert.
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Zero(t, h.calls)
		})
	}
	for _, key := range []string{"r_strings", "r_integers", "r_booleans", "o_strings", "o_integers", "o_booleans", "r_nullable_strings", "r_nullable_integers", "r_nullable_booleans", "o_nullable_strings", "o_nullable_integers", "o_nullable_booleans"} {
		t.Run("null item "+key, func(t *testing.T) {
			h := &captureHandler{}
			object := maps.Clone(base)
			object[key] = json.RawMessage("[null]")
			// Act.
			err := executeObject(t, h, object)
			// Assert.
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Zero(t, h.calls)
		})
	}
}
