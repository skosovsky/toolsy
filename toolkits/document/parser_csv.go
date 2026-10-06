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
// Final JSON is checked against the wire byte budget; oversized payloads return a limit error.
func parseCSV(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	return parseCSVWithLimits(
		ctx,
		r,
		Limits{SourceBytes: maxBytes, ParsedBytes: maxBytes, MaxItems: defaultMaxItems, ItemBytes: defaultItemBytes},
	)
}

func parseCSVWithLimits(ctx context.Context, r io.Reader, limits Limits) (string, error) {
	rows, err := readCSVRowsWithLimits(ctx, r, limits)
	if err != nil {
		return "", err
	}
	table, err := rowsToMarkdownTableBounded(ctx, rows, limits.ParsedBytes)
	if err != nil {
		return "", err
	}
	if limits.ParsedBytes > 0 && len(table) > limits.ParsedBytes {
		return "", toolsy.MapToolkitCapError(ctx, "document: csv table cap", limits.ParsedBytes, "csv table", "")
	}
	return table, nil
}

func readCSVRowsWithLimits(ctx context.Context, r io.Reader, limits Limits) ([][]string, error) {
	maxBytes := limits.SourceBytes
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
		if len(row) > limits.MaxItems {
			return nil, toolsy.NewValidationError("csv column count limit exceeded")
		}
		if len(rows) >= limits.MaxItems {
			return nil, toolsy.NewValidationError("csv row count limit exceeded")
		}
		for _, cell := range row {
			if len(cell) > limits.ItemBytes {
				return nil, toolsy.NewValidationError("csv cell byte limit exceeded")
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func rowsToMarkdownTable(ctx context.Context, rows [][]string) (string, error) {
	return rowsToMarkdownTableBounded(ctx, rows, defaultMaxBytes)
}

func rowsToMarkdownTableBounded(ctx context.Context, rows [][]string, limit int) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	var b strings.Builder
	if err := appendCSVRow(ctx, &b, rows[0], limit); err != nil {
		return "", err
	}
	for i := range len(rows[0]) {
		if i > 0 {
			b.WriteString(" | ")
		}
		if b.Len()+4 > limit {
			return "", toolsy.MapToolkitCapError(ctx, "document: csv table cap", limit, "csv table", "")
		}
		b.WriteString("---")
	}
	b.WriteString("\n")
	for _, row := range rows[1:] {
		if ie := toolsy.ToolkitContextError(ctx, "document: csv table"); ie != nil {
			return "", ie
		}
		if err := appendCSVRow(ctx, &b, row, limit); err != nil {
			return "", err
		}
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

func appendCSVRow(ctx context.Context, b *strings.Builder, row []string, limit int) error {
	for i, cell := range row {
		if err := toolsy.ToolkitContextError(ctx, "document: csv cell"); err != nil {
			return err
		}
		if i > 0 {
			b.WriteString(" | ")
		}
		escaped := escapeMarkdownCell(cell)
		if b.Len()+len(escaped)+1 > limit {
			return toolsy.MapToolkitCapError(ctx, "document: csv table cap", limit, "csv table", "")
		}
		b.WriteString(escaped)
	}
	b.WriteString("\n")
	return nil
}
