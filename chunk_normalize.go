package toolsy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

func validateChunk(c Chunk) error {
	if c.Event == "" {
		return NewInternalError(errors.New("toolsy: chunk event is required"))
	}
	if c.Event != EventProgress && c.Event != EventResult && c.Event != EventControl {
		return NewInternalError(fmt.Errorf("toolsy: unsupported chunk event %q", c.Event))
	}
	if err := validateControlDeclarations(c); err != nil {
		return err
	}
	if c.Event == EventControl {
		return nil
	}
	if c.Event == EventResult {
		if err := validateResultChunkAlgebra(c); err != nil {
			return err
		}
	}
	if c.IsError {
		return validateErrorChunk(c)
	}
	if len(c.Data) > 0 && c.MimeType == "" {
		return NewInternalError(fmt.Errorf("toolsy: chunk data requires mime type for event %q", c.Event))
	}
	if len(c.Data) == 0 && c.MimeType != "" {
		return NewInternalError(errors.New("toolsy: chunk mime type without data is invalid"))
	}
	if len(c.Data) > 0 && jsonMimeType(c.MimeType) {
		if _, err := jsonschemax.Decode(c.Data); err != nil {
			return NewInternalError(&ResultContractError{Kind: resultInvalidJSON, Cause: err})
		}
	}
	return nil
}

func validateResultChunkAlgebra(c Chunk) error {
	if c.IsError && (c.EmptyResult || c.Noop) {
		return invalidResultAlgebra("error result cannot declare Empty/Noop success status")
	}
	if err := validateResultFlags(c.EmptyResult, c.Noop, len(c.Data), len(c.Effects)); err != nil {
		return err
	}
	return nil
}

func validateResultFlags(empty, noop bool, wireBytes, effects int) error {
	switch {
	case empty && noop:
		return invalidResultAlgebra("Empty and Noop are exclusive")
	case (empty || noop) && wireBytes != 0:
		return invalidResultAlgebra("Empty/Noop result carries wire bytes")
	case noop && effects != 0:
		return invalidResultAlgebra("Noop cannot declare effects")
	default:
		return nil
	}
}

func invalidResultAlgebra(reason string) error {
	return NewInternalError(&ResultContractError{Kind: "result_algebra", Cause: errors.New(reason)})
}

func validateErrorChunk(c Chunk) error {
	if len(c.Data) == 0 {
		return NewInternalError(errors.New("toolsy: error chunks must include payload in Data"))
	}
	switch c.MimeType {
	case MimeTypeToolErrorJSON:
		if !json.Valid(c.Data) {
			return NewInternalError(errors.New("toolsy: tool error chunks must contain valid JSON"))
		}
	default:
		return NewInternalError(fmt.Errorf(
			"toolsy: error chunks require mime type %q",
			MimeTypeToolErrorJSON,
		))
	}
	return nil
}

// normalizeErrorChunk wraps legacy text (or other) error chunks in a structured ToolError envelope.
func normalizeErrorChunk(c Chunk) Chunk {
	if !c.IsError || c.MimeType == MimeTypeToolErrorJSON {
		return c
	}
	reason := "tool returned malformed error chunk: expected " + MimeTypeToolErrorJSON
	if detail := malformedErrorChunkDetail(c); detail != "" {
		reason += "; " + detail
	}
	return NewErrorChunkFromErr(&ToolError{ //nolint:exhaustruct_v5 // Err set below
		Code:      CodeInternal,
		Reason:    reason,
		Retryable: false,
		Err:       errors.New(reason),
	})
}

func malformedErrorChunkDetail(c Chunk) string {
	switch {
	case c.MimeType == MimeTypeText && len(c.Data) > 0:
		return strings.TrimSpace(string(c.Data))
	case c.MimeType != "":
		return fmt.Sprintf("unsupported mime type %q", c.MimeType)
	case len(c.Data) > 0:
		return strings.TrimSpace(string(c.Data))
	default:
		return ""
	}
}

// prepareChunk normalizes error chunks and validates the wire contract before delivery.
func prepareChunk(c Chunk) (Chunk, error) {
	if err := validateControlDeclarations(c); err != nil {
		return Chunk{}, err
	}
	if c.IsError {
		c = normalizeErrorChunk(c)
	}
	if err := validateChunk(c); err != nil {
		return Chunk{}, err
	}
	if c.Control != nil {
		c.Control = cloneControl(c.Control)
	}
	if len(c.Controls) != 0 {
		controls := make([]ControlSignal, len(c.Controls))
		for i, signal := range c.Controls {
			controls[i] = cloneControl(signal)
		}
		c.Controls = controls
	}
	if c.Envelope != nil {
		if !bytes.Equal(c.Envelope.Raw, c.Data) || c.Envelope.MimeType != c.MimeType {
			return Chunk{}, NewInternalError(&ResultContractError{Kind: "envelope_mismatch", Cause: nil})
		}
		if c.Event == EventResult &&
			((c.IsError && (c.Envelope.Kind != ToolEnvelopeKindError || c.Envelope.Error == nil)) ||
				(!c.IsError && (c.Envelope.Kind != ToolEnvelopeKindResult || c.Envelope.Error != nil))) {
			return Chunk{}, NewInternalError(&ResultContractError{Kind: "envelope_mismatch", Cause: nil})
		}
		c.Envelope = cloneToolEnvelope(c.Envelope)
	} else if c.Event == EventResult {
		envelope := c.ToolEnvelope()
		c.Envelope = &envelope
	}
	return c, nil
}
