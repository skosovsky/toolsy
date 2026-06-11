package toolsy

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
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
	case MimeTypeText:
		if !utf8.Valid(c.Data) {
			return NewInternalError(errors.New("toolsy: error chunks must contain valid UTF-8 text"))
		}
	case MimeTypeToolErrorJSON:
		if !json.Valid(c.Data) {
			return NewInternalError(errors.New("toolsy: tool error chunks must contain valid JSON"))
		}
	default:
		return NewInternalError(fmt.Errorf(
			"toolsy: error chunks require mime type %q or %q",
			MimeTypeText,
			MimeTypeToolErrorJSON,
		))
	}
	return nil
}
