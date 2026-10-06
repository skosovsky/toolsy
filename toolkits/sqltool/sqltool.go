package sqltool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
	"github.com/skosovsky/toolsy/internal/sqlutil"
	"github.com/skosovsky/toolsy/textprocessor"
)

type inspectArgs struct {
	TableNames []string `json:"table_names,omitempty"`
}

type InspectResult struct {
	Schema string `json:"schema"`
}

type executeArgs struct {
	Query string `json:"query"`
}

type ExecuteResult struct {
	Result    string `json:"result"`
	RowCount  int    `json:"row_count"`
	Truncated bool   `json:"truncated"`
}

const (
	truncationSuffix     = textprocessor.SQLRowsTruncationSuffix
	cellTruncationSuffix = textprocessor.SQLCellTruncationSuffix
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

	inspectTool, err := buildInspectTool(db, driverName, d, &o)
	if err != nil {
		return nil, fmt.Errorf("toolkit/sqltool: build inspect tool: %w", err)
	}

	executeTool, err := buildExecuteTool(db, &o)
	if err != nil {
		return nil, fmt.Errorf("toolkit/sqltool: build execute tool: %w", err)
	}
	return []toolsy.Tool{inspectTool, executeTool}, nil
}

func buildInspectTool(db *sql.DB, driverName string, d dialect, o *options) (toolsy.Tool, error) {
	if o.inspectFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[inspectArgs, format.JSONResult](
			o.inspectName,
			o.inspectDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args inspectArgs) (format.JSONResult, error) {
				res, err := doInspectSchema(ctx, db, driverName, d, o, args.TableNames)
				if err != nil {
					return format.JSONResult{}, err
				}
				raw, applyErr := format.ApplyWithEnvelope(
					res,
					func(v InspectResult) InspectResult { return v },
					o.inspectFormatter,
					o.hostResultValidator,
					o.maxSchemaBytes,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	}
	return toolsy.NewTool[inspectArgs, format.JSONResult](
		o.inspectName,
		o.inspectDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args inspectArgs) (format.JSONResult, error) {
			res, err := doInspectSchema(ctx, db, driverName, d, o, args.TableNames)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.ToJSONResult(res, o.maxSchemaBytes)
		},
		toolsy.WithReadOnly(),
	)
}

func buildExecuteTool(db *sql.DB, o *options) (toolsy.Tool, error) {
	if o.executeFormatter != nil || o.hostResultValidator != nil {
		return toolsy.NewTool[executeArgs, format.JSONResult](
			o.executeName,
			o.executeDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args executeArgs) (format.JSONResult, error) {
				res, err := doExecuteRead(ctx, db, o, args.Query)
				if err != nil {
					return format.JSONResult{}, err
				}
				raw, applyErr := format.ApplyWithEnvelope(
					res,
					func(v ExecuteResult) ExecuteResult { return v },
					o.executeFormatter,
					o.hostResultValidator,
					o.maxExecuteBytes,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolsy.WithReadOnly(),
		)
	}
	return toolsy.NewTool[executeArgs, format.JSONResult](
		o.executeName,
		o.executeDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args executeArgs) (format.JSONResult, error) {
			res, err := doExecuteRead(ctx, db, o, args.Query)
			if err != nil {
				return format.JSONResult{}, err
			}
			return format.ToJSONResult(res, o.maxExecuteBytes)
		},
		toolsy.WithReadOnly(),
	)
}

func doInspectSchema(
	ctx context.Context,
	db *sql.DB,
	driverName string,
	d dialect,
	o *options,
	tableNames []string,
) (InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return InspectResult{}, wrapSQLToolErr(err)
	}
	tables := tableNames
	if len(tables) > o.maxTables {
		return InspectResult{}, limitError("tables")
	}
	var err error
	if len(tables) == 0 {
		tables, err = fetchTableNamesFromDB(ctx, db, d, o)
		if err != nil {
			return InspectResult{}, err
		}
	} else if len(o.allowedTables) > 0 {
		tables = filterTablesByAllowlist(tables, o.allowedTables)
	}

	var b strings.Builder
	driverLower := strings.ToLower(driverName)
	fieldCount := 0
	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return InspectResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/sqltool: inspect schema: %w", err))
		}
		if err := appendColumnsToSchema(ctx, db, driverLower, d, table, &b, o, &fieldCount); err != nil {
			return InspectResult{}, err
		}
	}
	return InspectResult{Schema: strings.TrimSpace(b.String())}, nil
}

func fetchTableNamesFromDB(ctx context.Context, db *sql.DB, d dialect, o *options) ([]string, error) {
	rows, err := db.QueryContext(ctx, d.listTablesQuery())
	if err != nil {
		return nil, wrapSQLToolErr(fmt.Errorf("list tables: %w", err))
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	scanned, sourceBytes := 0, 0
	for rows.Next() {
		scanned++
		if scanned > o.maxTables {
			return nil, limitError("tables")
		}
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, wrapSQLToolErr(fmt.Errorf("scan table name: %w", scanErr))
		}
		sourceBytes += len(name)
		if sourceBytes > o.maxSourceBytes {
			return nil, limitError("source bytes")
		}
		if len(o.allowedTables) == 0 || contains(o.allowedTables, name) {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, wrapSQLToolErr(err)
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
	table string,
	b *strings.Builder,
	o *options,
	fieldCount *int,
) error {
	if b.Len()+len(table) > o.maxSourceBytes {
		return limitError("source bytes")
	}
	q, args := d.columnsQuery(table)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return wrapSQLToolErr(fmt.Errorf("columns for %s: %w", table, err))
	}
	defer func() { _ = rows.Close() }()

	var rowCount int
	for rows.Next() {
		if rowCount >= o.maxColumns || *fieldCount > o.maxCells-4 {
			return limitError("schema columns/cells")
		}
		*fieldCount += 4
		name, dataType, nullableStr, defaultVal, scanErr := scanColumnRow(rows, driverLower)
		if scanErr != nil {
			return wrapSQLToolErr(scanErr)
		}
		if b.Len()+len(name)+len(dataType)+len(nullableStr)+len(defaultVal) > o.maxSourceBytes {
			return limitError("source bytes")
		}
		if rowCount == 0 {
			fmt.Fprintf(
				b,
				"## Table: %s\n\n| Column | Type | Nullable | Default |\n|--------|------|----------|--------|\n",
				table,
			)
		}
		rowCount++
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s |\n",
			escapeMarkdownCell(name),
			escapeMarkdownCell(dataType),
			escapeMarkdownCell(nullableStr),
			escapeMarkdownCell(defaultVal),
		)
		if b.Len() > o.maxSourceBytes {
			return limitError("source bytes")
		}
	}
	if err := rows.Err(); err != nil {
		return wrapSQLToolErr(err)
	}
	if rowCount == 0 {
		fmt.Fprintf(b, "## Table: %s\n\nTable not found or has no columns.\n\n", table)
	} else {
		b.WriteString("\n")
	}
	if b.Len() > o.maxSourceBytes {
		return limitError("source bytes")
	}
	return nil
}

func scanColumnRow(rows *sql.Rows, driverLower string) (string, string, string, string, error) {
	var name, dataType, nullableStr, defaultVal string
	if driverLower == "sqlite3" || driverLower == "sqlite" {
		var cid, notnull, pk int64
		var dflt sql.NullString
		if scanErr := rows.Scan(&cid, &name, &dataType, &notnull, &dflt, &pk); scanErr != nil {
			return "", "", "", "", wrapSQLToolErr(fmt.Errorf("scan column: %w", scanErr))
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
		return "", "", "", "", wrapSQLToolErr(scanErr)
	}
	if def.Valid {
		defaultVal = def.String
	}
	return name, dataType, nullableStr, defaultVal, nil
}

func doExecuteRead(ctx context.Context, db *sql.DB, o *options, query string) (ExecuteResult, error) {
	query = strings.TrimSpace(query)
	if err := sqlutil.ValidateReadOnlyQuery(query); err != nil {
		return ExecuteResult{}, err
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return ExecuteResult{}, wrapSQLToolErr(fmt.Errorf("query: %w", err))
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return ExecuteResult{}, err
	}
	sourceBytes, err := checkColumns(cols, o)
	if err != nil {
		return ExecuteResult{}, err
	}
	var b strings.Builder
	writeMarkdownTableHeader(&b, cols)

	scanDest := make([]any, len(cols))
	vals := make([]sql.NullString, len(cols))
	for i := range vals {
		scanDest[i] = &vals[i]
	}
	rowCount := 0
	truncated := false
	for rows.Next() {
		if rowCount >= o.maxRows {
			b.WriteString(truncationSuffix)
			truncated = true
			break
		}
		if len(cols) > 0 && rowCount >= o.maxCells/len(cols) {
			return ExecuteResult{}, limitError("cells")
		}
		if err := rows.Scan(scanDest...); err != nil {
			return ExecuteResult{}, wrapSQLToolErr(err)
		}
		cellTruncated, consumeErr := consumeCells(vals, o, &sourceBytes)
		if consumeErr != nil {
			return ExecuteResult{}, consumeErr
		}
		truncated = truncated || cellTruncated
		appendMarkdownDataRow(&b, vals, o.maxCellBytes)
		if b.Len() > o.maxExecuteBytes {
			return ExecuteResult{}, limitError("execute bytes")
		}
		b.WriteString("\n")
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return ExecuteResult{}, wrapSQLToolErr(err)
	}
	return ExecuteResult{Result: strings.TrimSpace(b.String()), RowCount: rowCount, Truncated: truncated}, nil
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
			s = boundedCell(v.String, maxCellBytes)
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

func wrapSQLToolErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := toolsy.AsToolError(err); ok {
		return err
	}
	return toolsy.NewInternalError(fmt.Errorf("toolkit/sqltool: %w", err))
}

func limitError(kind string) error {
	return toolsy.NewValidationError(fmt.Sprintf("toolkit/sqltool: %s limit exceeded", kind))
}

// The byte limit includes the truncation marker and Markdown escaping.
func boundedCell(value string, limit int) string {
	escaped := escapeMarkdownCell(strings.ToValidUTF8(value, "�"))
	if len(escaped) <= limit {
		return escaped
	}
	suffix := cellTruncationSuffix
	if len(suffix) > limit {
		suffix = suffix[:limit]
	}
	return textprocessor.TruncateStringUTF8(escaped, limit-len(suffix), "") + suffix
}

func checkColumns(cols []string, o *options) (int, error) {
	if len(cols) > o.maxColumns {
		return 0, limitError("columns")
	}
	sourceBytes := 0
	for _, col := range cols {
		if len(col) > o.maxSourceBytes-sourceBytes {
			return 0, limitError("source bytes")
		}
		sourceBytes += len(col)
	}
	return sourceBytes, nil
}

func consumeCells(vals []sql.NullString, o *options, sourceBytes *int) (bool, error) {
	truncated := false
	for _, v := range vals {
		if len(v.String) > o.maxSourceBytes-*sourceBytes {
			return false, limitError("source bytes")
		}
		*sourceBytes += len(v.String)
		if len(escapeMarkdownCell(strings.ToValidUTF8(v.String, "�"))) > o.maxCellBytes {
			truncated = true
		}
	}
	return truncated, nil
}
