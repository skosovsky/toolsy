package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatContentResult is the result of FormatContent (formatted bytes + whether the tool reported isError).
type FormatContentResult struct {
	Data    []byte
	IsError bool
}

// FormatContent converts MCP content/contents (raw JSON from tools/call or resources/read)
// into LLM-friendly text: concatenates text parts and embeds images as Markdown
// ![image](data:<mediaType>;base64,<base64>). Exported so callers can override the formatting.
// rawResult may be a tools/call result (has "content" and optionally "isError") or
// resources/read result (has "contents"). Returns formatted bytes and isError flag, or error.
func FormatContent(rawResult []byte) (FormatContentResult, error) {
	var withContent struct {
		Content  []ContentItem `json:"content"`
		Contents []ContentItem `json:"contents"`
		IsError  bool          `json:"isError"`
	}
	if err := json.Unmarshal(rawResult, &withContent); err != nil {
		return FormatContentResult{}, err
	}
	items := withContent.Content
	if items == nil {
		items = withContent.Contents
	}
	out := formatContentItems(items)
	if withContent.IsError {
		if len(out) == 0 {
			out = []byte("Tool error")
		} else {
			out = []byte("Tool error: " + string(out))
		}
	}
	return FormatContentResult{Data: out, IsError: withContent.IsError}, nil
}

// formatContentItems builds a single text from content items (text + Markdown image links).
func formatContentItems(items []ContentItem) []byte {
	var b strings.Builder
	for i, item := range items {
		if i > 0 {
			b.WriteString("\n")
		}
		switch item.Type {
		case "text":
			b.WriteString(item.Text)
		case "image":
			mediaType := item.MediaType
			if mediaType == "" {
				mediaType = "image/png"
			}
			_, _ = fmt.Fprintf(&b, "![image](data:%s;base64,%s)", mediaType, item.Base64)
		default:
			if item.Text != "" {
				b.WriteString(item.Text)
			}
		}
	}
	return []byte(b.String())
}

// FormatContentItems is the exported formatter for a slice of ContentItem.
// Use this to customize how tools/call or resources/read content is presented to the LLM.
func FormatContentItems(items []ContentItem) []byte {
	return formatContentItems(items)
}
