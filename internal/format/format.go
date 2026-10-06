package format

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

// WireContentCap returns the content byte budget derived from a wire JSON budget and fixed envelope overhead.
// Use when transport reads must leave room for JSON field names and envelope bytes (before ValidateWireJSON).
func WireContentCap(maxWireBytes, envelopeOverhead int) int {
	if maxWireBytes <= envelopeOverhead {
		return maxWireBytes
	}
	return maxWireBytes - envelopeOverhead
}

// Apply runs optional formatter and host validator on a typed value, returning JSON bytes.
func Apply[T any](
	value T,
	formatter func(T) (any, error),
	validator func(any) error,
) (json.RawMessage, error) {
	return ApplyWithEnvelope(value, func(v T) T { return v }, formatter, validator, 0)
}

// ApplyWithEnvelope runs formatter on value (if set), otherwise envelope(value), then validator, then JSON marshal.
// A positive maxWireBytes rejects oversized JSON without modifying its representation.
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
			return nil, toolsy.NewInternalError(&toolsy.ResultContractError{Kind: "result_validator", Cause: err})
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("internal/format: marshal result: %w", err))
	}
	return ValidateWireJSON(data, maxWireBytes)
}

// MarshalWireCap marshals v and rejects JSON exceeding a positive wire byte budget.
func MarshalWireCap(v any, maxWireBytes int) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("internal/format: marshal result: %w", err))
	}
	return ValidateWireJSON(data, maxWireBytes)
}

// ToJSONResult marshals v with an optional wire byte budget into a JSONResult.
func ToJSONResult(v any, maxWireBytes int) (JSONResult, error) {
	raw, err := MarshalWireCap(v, maxWireBytes)
	if err != nil {
		return JSONResult{}, err
	}
	return JSONResult{Raw: raw}, nil
}

// WireLimitError reports the exact encoded size and configured wire limit.
// Callers can use [errors.As] through the returned ToolError to inspect these bounds.
type WireLimitError struct {
	Size  int
	Limit int
}

func (e *WireLimitError) Error() string {
	return fmt.Sprintf("JSON result exceeds wire byte limit: %d > %d", e.Size, e.Limit)
}

// Unwrap preserves the validation error category for oversized results.
func (e *WireLimitError) Unwrap() error {
	return toolsy.ErrValidation
}

// ValidateWireJSON rejects invalid or oversized JSON. A nonpositive cap is unlimited.
// It never slices serialized JSON or manufactures a replacement result.
func ValidateWireJSON(raw json.RawMessage, maxBytes int) (json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, toolsy.NewInternalError(errors.New("invalid JSON result"))
	}
	if maxBytes > 0 && len(raw) > maxBytes {
		limitErr := &WireLimitError{Size: len(raw), Limit: maxBytes}
		err := toolsy.NewValidationError(limitErr.Error())
		err.Err = limitErr
		return nil, err
	}
	return raw, nil
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
// Invalid JSON is rejected; nil Raw encodes as JSON null.
func (j JSONResult) MarshalJSON() ([]byte, error) {
	if j.Raw == nil {
		return []byte("null"), nil
	}
	return ValidateWireJSON(j.Raw, 0)
}
