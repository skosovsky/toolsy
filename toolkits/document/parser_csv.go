package document

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

// parseCSV reads CSV from r and returns a Markdown table. Content is capped without a truncation suffix.
func parseCSV(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	rows, err := readCSVRows(ctx, r)
	if err != nil {
		return "", err
	}
	return rowsToMarkdownTable(ctx, rows, maxBytes), nil
}

func readCSVRows(ctx context.Context, r io.Reader) ([][]string, error) {
	rd := csv.NewReader(r)
	var rows [][]string
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("toolkit/document: %w", err)
		}
		row, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, toolsy.NewInternalError(fmt.Errorf("document: csv read: %w", err))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func rowsToMarkdownTable(ctx context.Context, rows [][]string, maxBytes int) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, cell := range rows[0] {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(escapeMarkdownCell(cell))
	}
	b.WriteString("\n")
	for i := range len(rows[0]) {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		if err := ctx.Err(); err != nil {
			return textprocessor.TruncateStringUTF8NoSuffix(b.String(), maxBytes)
		}
		for i, cell := range row {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(escapeMarkdownCell(cell))
		}
		b.WriteString("\n")
		if b.Len() > maxBytes {
			return textprocessor.TruncateStringUTF8NoSuffix(b.String(), maxBytes)
		}
	}
	return textprocessor.TruncateStringUTF8NoSuffix(b.String(), maxBytes)
}

// escapeMarkdownCell escapes pipe and normalizes newlines so multiline cells do not break the Markdown table.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}
