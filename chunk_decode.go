package toolsy

import (
	"encoding/json"
	"fmt"
)

// DecodeChunkAs unmarshals a JSON result chunk into T after validating event and mime type.
func DecodeChunkAs[T any](c Chunk) (*T, error) {
	if c.Event != EventResult {
		return nil, NewSchemaError(fmt.Sprintf("cannot decode event %q into requested type", c.Event))
	}
	if c.IsError {
		return nil, NewSchemaError(
			"cannot decode error result chunk into struct; use DecodeChunkAsText or read chunk Data",
		)
	}
	if c.MimeType != MimeTypeJSON {
		return nil, NewSchemaError(fmt.Sprintf("cannot decode %q into requested type", c.MimeType))
	}
	var out T
	if err := json.Unmarshal(c.Data, &out); err != nil {
		return nil, NewJSONParseError(err)
	}
	return &out, nil
}

// DecodeChunkAsText returns plain-text payload from a result chunk.
func DecodeChunkAsText(c Chunk) (string, error) {
	if c.Event != EventResult {
		return "", NewSchemaError(fmt.Sprintf("cannot decode event %q as text", c.Event))
	}
	if c.IsError {
		return "", NewSchemaError("cannot decode error result chunk as text; read chunk Data directly")
	}
	if c.MimeType != MimeTypeText {
		return "", NewSchemaError(fmt.Sprintf("cannot decode %q as text", c.MimeType))
	}
	return string(c.Data), nil
}
