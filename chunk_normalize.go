package toolsy

import (
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
		if len(c.Data) == 0 {
			return NewInternalError(errors.New("toolsy: error chunks must include UTF-8 text in Data"))
		}
		if c.MimeType != MimeTypeText {
			return NewInternalError(fmt.Errorf("toolsy: error chunks require mime type %q", MimeTypeText))
		}
		if !utf8.Valid(c.Data) {
			return NewInternalError(errors.New("toolsy: error chunks must contain valid UTF-8 text"))
		}
		return nil
	}
	if len(c.Data) > 0 && c.MimeType == "" {
		return NewInternalError(fmt.Errorf("toolsy: chunk data requires mime type for event %q", c.Event))
	}
	if len(c.Data) == 0 && c.MimeType != "" {
		return NewInternalError(errors.New("toolsy: chunk mime type without data is invalid"))
	}
	return nil
}
