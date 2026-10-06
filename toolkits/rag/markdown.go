package rag

import (
	"fmt"
	"strings"
)

// FormatDocumentsMarkdown renders documents as numbered Markdown for LLM consumption.
func FormatDocumentsMarkdown(docs []Document) string {
	if len(docs) == 0 {
		return "No results found."
	}
	var b strings.Builder
	n := 1
	for _, doc := range docs {
		content := strings.TrimSpace(doc.Content)
		if content == "" {
			content = "(empty content)"
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		source := strings.TrimSpace(doc.SourceURI)
		if source == "" {
			source = "unavailable"
		}
		_, _ = fmt.Fprintf(&b, "%d. %s\n   Source: %s", n, content, source)
		if doc.ID != "" {
			_, _ = fmt.Fprintf(&b, "\n   ID: %s", doc.ID)
		}
		n++
	}
	if b.Len() == 0 {
		return "No results found."
	}
	return b.String()
}
