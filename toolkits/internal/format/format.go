package format

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

// Apply runs optional formatter and host validator on a typed value, returning JSON bytes.
func Apply[T any](
	value T,
	formatter func(T) (any, error),
	validator func(any) error,
) (json.RawMessage, error) {
	return ApplyWithEnvelope(value, func(v T) T { return v }, formatter, validator, 0)
}

// ApplyWithEnvelope runs formatter on value (if set), otherwise envelope(value), then validator, then JSON marshal.
// When maxWireBytes > 0, the marshaled wire JSON is UTF-8 truncated with textprocessor.TruncationSuffix.
// Use when validator-only mode must validate the default tool wire shape, not the raw typed value.
func ApplyWithEnvelope[T any, E any](
	value T,
	envelope func(T) E,
	formatter func(T) (any, error),
	validator func(any) error,
	maxWireBytes int,
) (json.RawMessage, error) {
	var out any
	if formatter != nil {
		var err error
		out, err = formatter(value)
		if err != nil {
			return nil, err
		}
	} else {
		out = envelope(value)
	}
	if validator != nil {
		if err := validator(out); err != nil {
			return nil, validationError(err)
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("toolkit/format: marshal result: %w", err)
	}
	return CapWireJSON(data, maxWireBytes, textprocessor.TruncationSuffix), nil
}

// MarshalWireCap marshals v to JSON and caps the wire bytes to maxWireBytes.
func MarshalWireCap(v any, maxWireBytes int) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("toolkit/format: marshal result: %w", err)
	}
	return CapWireJSON(data, maxWireBytes, textprocessor.TruncationSuffix), nil
}

// ToJSONResult marshals v with an optional wire byte budget into a JSONResult.
func ToJSONResult(v any, maxWireBytes int) (JSONResult, error) {
	raw, err := MarshalWireCap(v, maxWireBytes)
	if err != nil {
		return JSONResult{}, err
	}
	return JSONResult{Raw: raw}, nil
}

// CapWireJSON truncates marshaled wire JSON to maxBytes with a UTF-8 safe suffix.
// When maxBytes <= 0 or len(raw) <= maxBytes, raw is returned unchanged.
func CapWireJSON(raw json.RawMessage, maxBytes int, suffix string) json.RawMessage {
	if maxBytes <= 0 || len(raw) <= maxBytes {
		return raw
	}
	if suffix == "" {
		suffix = textprocessor.TruncationSuffix
	}
	capped := textprocessor.TruncateBytesToValidUTF8String(raw, maxBytes, suffix)
	return json.RawMessage(capped)
}

func validationError(err error) error {
	var te *toolsy.ToolError
	if errors.As(err, &te) {
		return te
	}
	return toolsy.NewValidationError(err.Error())
}

// JSONResult wraps pre-marshaled JSON for toolsy.NewTool without double-encoding.
type JSONResult struct {
	Raw json.RawMessage
}

// WireJSON implements [toolsy.WireJSONResult].
func (j JSONResult) WireJSON() json.RawMessage {
	return j.Raw
}

// MarshalJSON implements [json.Marshaler].
// Raw wire bytes are returned as-is (valid or truncated); nil Raw encodes as JSON null.
func (j JSONResult) MarshalJSON() ([]byte, error) {
	if j.Raw == nil {
		return []byte("null"), nil
	}
	return j.Raw, nil
}
