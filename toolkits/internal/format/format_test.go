package format

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
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

func TestApplyWithEnvelope_ValidatorReject_CodeValidationFailed(t *testing.T) {
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
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestCapWireJSON_TruncatesOversizedWire(t *testing.T) {
	raw := json.RawMessage(`{"payload":"` + strings.Repeat("x", 200) + `"}`)
	capped := CapWireJSON(raw, 50, textprocessor.TruncationSuffix)
	require.LessOrEqual(t, len(capped), 50+len(textprocessor.TruncationSuffix)+2)
	require.Contains(t, string(capped), "[Truncated]")
}

func TestApplyWithEnvelope_MaxWireBytes(t *testing.T) {
	raw, err := ApplyWithEnvelope(
		"hello",
		func(s string) map[string]string { return map[string]string{"text": s} },
		func(string) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		},
		nil,
		80,
	)
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestMarshalWireCap_TruncatesWire(t *testing.T) {
	raw, err := MarshalWireCap(map[string]string{"blob": strings.Repeat("a", 200)}, 40)
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw), 40+len(textprocessor.TruncationSuffix)+2)
}

func TestJSONResult_MarshalJSON_InvalidWire(t *testing.T) {
	raw := json.RawMessage(`{"key":"value`)
	jr := JSONResult{Raw: raw}
	data, err := jr.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, string(raw), string(data))
	require.False(t, json.Valid(data))
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
