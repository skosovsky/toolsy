package toolsy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func validateChunk(c Chunk) error {
	if c.Event == "" {
		return NewInternalError(errors.New("toolsy: chunk event is required"))
	}
	if c.Event != EventProgress && c.Event != EventResult && c.Event != EventControl {
		return NewInternalError(fmt.Errorf("toolsy: unsupported chunk event %q", c.Event))
	}
	if c.Event == EventControl {
		if c.Control == nil {
			return NewInternalError(errors.New("toolsy: control chunk requires typed Control signal"))
		}
		return nil
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
	return nil
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
	return NewErrorChunkFromErr(&ToolError{ //nolint:exhaustruct // Err set below
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
	if c.IsError {
		c = normalizeErrorChunk(c)
	}
	if err := validateChunk(c); err != nil {
		return Chunk{}, err
	}
	if c.Envelope != nil {
		c.Envelope = cloneToolEnvelope(c.Envelope)
	} else if c.Event == EventResult {
		envelope := c.ToolEnvelope()
		c.Envelope = &envelope
	}
	return c, nil
}
