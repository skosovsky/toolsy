package format

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestApply_ValidatorOnly(t *testing.T) {
	raw, err := Apply("hello", nil, func(v any) error {
		s, ok := v.(string)
		if !ok || s != "hello" {
			return errors.New("unexpected")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, `"hello"`, string(raw))
}

func TestApplyWithEnvelope_ValidatorOnly(t *testing.T) {
	type envelope struct {
		Text string `json:"text"`
	}
	raw, err := ApplyWithEnvelope(
		"hello",
		func(s string) envelope { return envelope{Text: s} },
		nil,
		func(v any) error {
			e, ok := v.(envelope)
			if !ok || e.Text != "hello" {
				return errors.New("expected envelope")
			}
			return nil
		},
		0,
	)
	require.NoError(t, err)
	var got envelope
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, "hello", got.Text)
}

func TestApplyWithEnvelope_FormatterError(t *testing.T) {
	_, err := ApplyWithEnvelope(
		1,
		func(n int) map[string]int { return map[string]int{"n": n} },
		func(int) (any, error) { return nil, errors.New("fmt err") },
		nil,
		0,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fmt err")
}

func TestApplyWithEnvelope_NilEnvelopeUsesFormatter(t *testing.T) {
	raw, err := ApplyWithEnvelope(
		[]int{1, 2},
		func(v []int) map[string]int { return map[string]int{"count": len(v)} },
		func(v []int) (any, error) { return map[string]int{"items": len(v)}, nil },
		nil,
		0,
	)
	require.NoError(t, err)
	require.JSONEq(t, `{"items":2}`, string(raw))
}

func TestApplyWithEnvelope_ValidatorReject_ResultContract(t *testing.T) {
	_, err := ApplyWithEnvelope(
		"hello",
		func(s string) string { return s },
		nil,
		func(_ any) error { return errors.New("reject") },
		0,
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestWireContentCap(t *testing.T) {
	require.Equal(t, 984, WireContentCap(1000, 16))
	require.Equal(t, 1000, WireContentCap(1000, 0))
	require.Equal(t, 5, WireContentCap(20, 15))
}

func TestValidateWireJSON_BoundsAndValidity(t *testing.T) {
	for _, value := range []any{"Привет", "\"\n<>", map[string]any{"nested": []any{1, true, "text"}}, nil} {
		// Arrange
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		for _, cap := range []int{0, 1, len(raw) - 1, len(raw), len(raw) + 1} {
			// Act
			got, err := ValidateWireJSON(raw, cap)
			// Assert
			if cap > 0 && len(raw) > cap {
				require.Error(t, err)
				require.Nil(t, got)
				var limit *WireLimitError
				require.ErrorAs(t, err, &limit)
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Equal(t, len(raw), limit.Size)
				require.Equal(t, cap, limit.Limit)
			} else {
				require.NoError(t, err)
				require.True(t, json.Valid(got))
				require.JSONEq(t, string(raw), string(got))
				require.Len(t, got, len(raw))
			}
		}
	}
}

func TestValidateWireJSON_Invalid(t *testing.T) {
	// Arrange
	raw := json.RawMessage(`{"bad":`)
	// Act
	got, err := ValidateWireJSON(raw, 0)
	// Assert
	require.Error(t, err)
	require.Nil(t, got)
}

func TestApplyWithEnvelope_MaxWireBytes(t *testing.T) {
	// Arrange
	value := strings.Repeat("z", 500)
	// Act
	raw, err := ApplyWithEnvelope(value, func(s string) string { return s }, nil, nil, 80)
	// Assert
	require.Error(t, err)
	require.Nil(t, raw)
	var limit *WireLimitError
	require.ErrorAs(t, err, &limit)
}

func TestMarshalWireCap_RejectsOversizedWire(t *testing.T) {
	// Arrange
	value := map[string]string{"blob": strings.Repeat("a", 200)}
	// Act
	raw, err := MarshalWireCap(value, 40)
	// Assert
	require.Error(t, err)
	require.Nil(t, raw)
}

func TestJSONResult_MarshalJSON_InvalidWire(t *testing.T) {
	raw := json.RawMessage(`{"key":"value`)
	jr := JSONResult{Raw: raw}
	data, err := jr.MarshalJSON()
	require.Error(t, err)
	require.Nil(t, data)
}

func TestJSONResult_MarshalJSON_Nil(t *testing.T) {
	jr := JSONResult{}
	data, err := jr.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, "null", string(data))
}

func TestJSONResult_MarshalJSON_ValidPassthrough(t *testing.T) {
	raw := json.RawMessage(`{"ok":true,"n":3}`)
	jr := JSONResult{Raw: raw}
	data, err := jr.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, string(raw), string(data))
}

func TestHostValidatorPreservesPostHandlerCause(t *testing.T) {
	// Arrange.
	cause := toolsy.NewValidationError("repair", "query")
	// Act.
	_, err := ApplyWithEnvelope("value", func(s string) string { return s }, nil, func(any) error { return cause }, 0)
	// Assert.
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.False(t, toolsy.ClientCorrectable(te.Code))
	require.False(t, te.Retryable)
	require.Empty(t, te.FixableArgs)
	require.ErrorIs(t, err, cause)
	var contract *toolsy.ResultContractError
	require.ErrorAs(t, err, &contract)
	require.Equal(t, "result_validator", contract.Kind)
}
