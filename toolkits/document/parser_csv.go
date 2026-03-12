package document

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// parseCSV reads CSV from r and returns a Markdown table. Truncates to maxBytes (UTF-8 safe).
func parseCSV(r io.Reader, maxBytes int) (string, error) {
	rd := csv.NewReader(r)
	rows, err := rd.ReadAll()
	if err != nil {
		return "", fmt.Errorf("document: csv read: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	var b strings.Builder
	// Header row
	for i, cell := range rows[0] {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(escapeMarkdownCell(cell))
	}
	b.WriteString("\n")
	// Separator
	for i := 0; i < len(rows[0]); i++ {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString("\n")
	// Data rows
	for _, row := range rows[1:] {
		for i, cell := range row {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(escapeMarkdownCell(cell))
		}
		b.WriteString("\n")
		if b.Len() > maxBytes {
			return truncateUTF8(b.String(), maxBytes), nil
		}
	}
	return truncateUTF8(b.String(), maxBytes), nil
}

// escapeMarkdownCell escapes pipe and normalizes newlines so multiline cells do not break the Markdown table.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}
