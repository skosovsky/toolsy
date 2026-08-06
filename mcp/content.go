package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func canonicalJSON(raw json.RawMessage) ([]byte, any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("mcp: multiple JSON values")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return canonical, value, nil
}

func formatContentBlocks(blocks []ContentBlock) ([]byte, error) {
	var out strings.Builder
	for index, block := range blocks {
		if index > 0 {
			out.WriteByte('\n')
		}
		if err := block.validate(); err != nil {
			return nil, err
		}
		switch block.Type {
		case contentTypeText:
			out.WriteString(block.Text)
		case contentTypeImage:
			fmt.Fprintf(&out, "![image](data:%s;base64,%s)", block.MIMEType, block.Data)
		case contentTypeAudio:
			fmt.Fprintf(&out, "[audio](data:%s;base64,%s)", block.MIMEType, block.Data)
		case contentTypeResourceLink:
			if block.URI == "" {
				return nil, errors.New("mcp: resource_link requires uri and name")
			}
			fmt.Fprintf(&out, "[%s](%s)", block.Name, block.URI)
		case contentTypeResource:
			if block.Resource == nil {
				return nil, errors.New("mcp: embedded resource requires resource")
			}
			projection, err := formatResourceContents([]ResourceContents{*block.Resource})
			if err != nil {
				return nil, err
			}
			out.Write(projection)
		default:
			return nil, fmt.Errorf("mcp: unsupported content type %q", block.Type)
		}
	}
	return []byte(out.String()), nil
}

func formatResourceContents(contents []ResourceContents) ([]byte, error) {
	var out strings.Builder
	for index, content := range contents {
		if err := content.validate(); err != nil {
			return nil, err
		}
		if index > 0 {
			out.WriteByte('\n')
		}
		switch {
		case content.Text != nil && content.Blob == nil:
			out.WriteString(*content.Text)
		case content.Blob != nil && content.Text == nil:
			mimeType := content.MIMEType
			if mimeType == "" {
				mimeType = applicationOctetStream
			}
			fmt.Fprintf(&out, "[resource](data:%s;base64,%s)", mimeType, *content.Blob)
		default:
			return nil, fmt.Errorf(
				"mcp: resource %q must contain exactly one of text or blob",
				content.URI,
			)
		}
	}
	return []byte(out.String()), nil
}

// FormatContentBlocks creates the deterministic LLM-facing projection while callers
// retain the original typed blocks separately.
func FormatContentBlocks(blocks []ContentBlock) ([]byte, error) {
	return formatContentBlocks(blocks)
}

// FormatResourceContents creates the deterministic LLM-facing projection of resources.
func FormatResourceContents(contents []ResourceContents) ([]byte, error) {
	return formatResourceContents(contents)
}
