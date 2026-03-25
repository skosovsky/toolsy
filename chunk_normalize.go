package toolsy

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

func normalizeChunk(c Chunk) (Chunk, error) {
	if c.Event == "" {
		c.Event = EventResult
	}
	if c.IsError {
		if c.Data == nil && c.RawData != nil {
			return Chunk{}, &SystemError{
				Err: errors.New("toolsy: error chunks must set Data as UTF-8 text; RawData is unsupported"),
			}
		}
		if c.Data != nil && c.MimeType == "" {
			c.MimeType = MimeTypeText
		}
		if c.Data != nil && c.MimeType != MimeTypeText {
			return Chunk{}, &SystemError{Err: fmt.Errorf("toolsy: error chunks require mime type %q", MimeTypeText)}
		}
		if c.Data != nil && !utf8.Valid(c.Data) {
			return Chunk{}, &SystemError{
				Err: errors.New("toolsy: error chunks must contain valid UTF-8 text"),
			}
		}
	}
	if c.RawData != nil && c.Data == nil {
		data, err := json.Marshal(c.RawData)
		if err != nil {
			return Chunk{}, &SystemError{Err: fmt.Errorf("toolsy: marshal chunk raw data: %w", err)}
		}
		c.Data = data
		c.MimeType = MimeTypeJSON
	}
	if c.Data != nil && c.MimeType == "" {
		return Chunk{}, &SystemError{Err: fmt.Errorf("toolsy: chunk data requires mime type for event %q", c.Event)}
	}
	return c, nil
}
