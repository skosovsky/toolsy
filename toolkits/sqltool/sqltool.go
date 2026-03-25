package sqltool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/sqlutil"
	"github.com/skosovsky/toolsy/internal/textutil"
)

type inspectArgs struct {
	TableNames []string `json:"table_names,omitempty"`
}

type inspectResult struct {
	Schema string `json:"schema"`
}

type executeArgs struct {
	Query string `json:"query"`
}

type executeResult struct {
	Result   string `json:"result"`
	RowCount int    `json:"row_count"`
}

const (
	truncationSuffix     = "\n[Truncated: max rows reached]"
	cellTruncationSuffix = "..."
)

// AsTools returns sql_inspect_schema and sql_execute_read tools. db must use a read-only user in production.
func AsTools(db *sql.DB, driverName string, opts ...Option) ([]toolsy.Tool, error) {
	if db == nil {
		return nil, errors.New("toolkit/sqltool: db is nil")
	}
	d, err := newDialect(driverName)
	if err != nil {
		return nil, err
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	inspectTool, err := toolsy.NewTool[inspectArgs, inspectResult](
		o.inspectName,
		o.inspectDesc,
		func(ctx context.Context, args inspectArgs) (inspectResult, error) {
			return doInspectSchema(ctx, db, driverName, d, &o, args.TableNames)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/sqltool: build inspect tool: %w", err)
	}

	executeTool, err := toolsy.NewTool[executeArgs, executeResult](
		o.executeName,
		o.executeDesc,
		func(ctx context.Context, args executeArgs) (executeResult, error) {
			return doExecuteRead(ctx, db, &o, args.Query)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/sqltool: build execute tool: %w", err)
	}
	return []toolsy.Tool{inspectTool, executeTool}, nil
}

func doInspectSchema(
	ctx context.Context,
	db *sql.DB,
	driverName string,
	d dialect,
	o *options,
	tableNames []string,
) (inspectResult, error) {
	tables := tableNames
	var err error
	if len(tables) == 0 {
		tables, err = fetchTableNamesFromDB(ctx, db, d, o)
		if err != nil {
			return inspectResult{}, err
		}
	} else if len(o.allowedTables) > 0 {
		tables = filterTablesByAllowlist(tables, o.allowedTables)
	}

	var b strings.Builder
	driverLower := strings.ToLower(driverName)
	const schemaTruncatedSuffix = "\n\n[Truncated: schema output limit reached]"
	for _, table := range tables {
		if b.Len() >= o.maxSchemaBytes {
			b.WriteString(schemaTruncatedSuffix)
			break
		}
		if err := ctx.Err(); err != nil {
			return inspectResult{}, fmt.Errorf("toolkit/sqltool: %w", err)
		}
		if err := appendColumnsToSchema(ctx, db, driverLower, d, o, table, &b); err != nil {
			return inspectResult{}, err
		}
	}
	return inspectResult{Schema: strings.TrimSpace(b.String())}, nil
}

func fetchTableNamesFromDB(ctx context.Context, db *sql.DB, d dialect, o *options) ([]string, error) {
	rows, err := db.QueryContext(ctx, d.listTablesQuery())
	if err != nil {
		return nil, fmt.Errorf("toolkit/sqltool: list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("toolkit/sqltool: scan table name: %w", scanErr)
		}
		if len(o.allowedTables) == 0 || contains(o.allowedTables, name) {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func filterTablesByAllowlist(tables, allowed []string) []string {
	filtered := make([]string, 0, len(tables))
	for _, t := range tables {
		if contains(allowed, t) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func appendColumnsToSchema(
	ctx context.Context,
	db *sql.DB,
	driverLower string,
	d dialect,
	o *options,
	table string,
	b *strings.Builder,
) error {
	q, args := d.columnsQuery(table)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("toolkit/sqltool: columns for %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var rowCount int
	for rows.Next() {
		name, dataType, nullableStr, defaultVal, scanErr := scanColumnRow(rows, driverLower)
		if scanErr != nil {
			return scanErr
		}
		if rowCount == 0 {
			fmt.Fprintf(
				b,
				"## Table: %s\n\n| Column | Type | Nullable | Default |\n|--------|------|----------|--------|\n",
				table,
			)
		}
		rowCount++
		if b.Len() < o.maxSchemaBytes {
			fmt.Fprintf(b, "| %s | %s | %s | %s |\n", name, dataType, nullableStr, defaultVal)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if rowCount == 0 {
		fmt.Fprintf(b, "## Table: %s\n\nTable not found or has no columns.\n\n", table)
	} else {
		b.WriteString("\n")
	}
	return nil
}

func scanColumnRow(rows *sql.Rows, driverLower string) (string, string, string, string, error) {
	var name, dataType, nullableStr, defaultVal string
	if driverLower == "sqlite3" || driverLower == "sqlite" {
		var cid, notnull, pk int64
		var dflt sql.NullString
		if scanErr := rows.Scan(&cid, &name, &dataType, &notnull, &dflt, &pk); scanErr != nil {
			return "", "", "", "", fmt.Errorf("toolkit/sqltool: scan column: %w", scanErr)
		}
		if notnull == 0 {
			nullableStr = "YES"
		} else {
			nullableStr = "NO"
		}
		if dflt.Valid {
			defaultVal = dflt.String
		}
		return name, dataType, nullableStr, defaultVal, nil
	}
	var def sql.NullString
	if scanErr := rows.Scan(&name, &dataType, &nullableStr, &def); scanErr != nil {
		return "", "", "", "", scanErr
	}
	if def.Valid {
		defaultVal = def.String
	}
	return name, dataType, nullableStr, defaultVal, nil
}

func doExecuteRead(ctx context.Context, db *sql.DB, o *options, query string) (executeResult, error) {
	query = strings.TrimSpace(query)
	if err := sqlutil.ValidateReadOnlyQuery(query); err != nil {
		return executeResult{}, err
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return executeResult{}, fmt.Errorf("toolkit/sqltool: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return executeResult{}, err
	}
	var b strings.Builder
	writeMarkdownTableHeader(&b, cols)

	scanDest := make([]any, len(cols))
	vals := make([]sql.NullString, len(cols))
	for i := range vals {
		scanDest[i] = &vals[i]
	}
	rowCount := 0
	for rows.Next() {
		if rowCount >= o.maxRows {
			b.WriteString(truncationSuffix)
			break
		}
		if err := rows.Scan(scanDest...); err != nil {
			return executeResult{}, err
		}
		appendMarkdownDataRow(&b, vals, o.maxCellBytes)
		b.WriteString("\n")
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return executeResult{}, err
	}
	return executeResult{Result: strings.TrimSpace(b.String()), RowCount: rowCount}, nil
}

func writeMarkdownTableHeader(b *strings.Builder, cols []string) {
	for i, c := range cols {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(escapeMarkdownCell(c))
	}
	b.WriteString("\n")
	for i := range cols {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString("\n")
}

func appendMarkdownDataRow(b *strings.Builder, vals []sql.NullString, maxCellBytes int) {
	for i, v := range vals {
		if i > 0 {
			b.WriteString(" | ")
		}
		s := ""
		if v.Valid {
			s = escapeMarkdownCell(textutil.TruncateStringUTF8(v.String, maxCellBytes, cellTruncationSuffix))
		}
		b.WriteString(s)
	}
}

// escapeMarkdownCell escapes pipe and newlines so table cells do not break Markdown.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func contains(slice []string, s string) bool {
	return slices.Contains(slice, s)
}
