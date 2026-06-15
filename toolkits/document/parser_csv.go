package document

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

// parseCSV reads CSV from r with a fail-closed byte budget (ReadLimitedBytes), then builds a Markdown table.
// Wire truncation applies only on final JSON marshal via format.CapWireJSON.
func parseCSV(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	rows, err := readCSVRows(ctx, r, maxBytes)
	if err != nil {
		return "", err
	}
	table, err := rowsToMarkdownTable(ctx, rows)
	if err != nil {
		return "", err
	}
	if maxBytes > 0 && len(table) > maxBytes {
		return "", toolsy.MapToolkitCapError(ctx, "document: csv table cap", maxBytes, "csv table", "")
	}
	return table, nil
}

func readCSVRows(ctx context.Context, r io.Reader, maxBytes int) ([][]string, error) {
	data, err := textprocessor.ReadLimitedBytes(ctx, r, maxBytes)
	if mapped := toolsy.MapToolkitReadError(ctx, err, "document: csv read", maxBytes, "csv", ""); mapped != nil {
		return nil, mapped
	}
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("document: csv read: %w", err))
	}
	rd := csv.NewReader(bytes.NewReader(data))
	var rows [][]string
	for {
		if ie := toolsy.ToolkitContextError(ctx, "document: csv row"); ie != nil {
			return nil, ie
		}
		row, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, toolsy.NewInternalError(fmt.Errorf("document: csv parse: %w", err))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func rowsToMarkdownTable(ctx context.Context, rows [][]string) (string, error) {
	if len(rows) == 0 {
		return "", nil
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
		if ie := toolsy.ToolkitContextError(ctx, "document: csv table"); ie != nil {
			return "", ie
		}
		for i, cell := range row {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(escapeMarkdownCell(cell))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// escapeMarkdownCell escapes pipe and normalizes newlines so multiline cells do not break the Markdown table.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}
