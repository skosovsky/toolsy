package sqltool

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
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

const truncationSuffix = "\n[Truncated: max rows reached]"

// AsTools returns sql_inspect_schema and sql_execute_read tools. db must use a read-only user in production.
func AsTools(db *sql.DB, driverName string, opts ...Option) ([]toolsy.Tool, error) {
	if db == nil {
		return nil, fmt.Errorf("toolkit/sqltool: db is nil")
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

func doInspectSchema(ctx context.Context, db *sql.DB, driverName string, d dialect, o *options, tableNames []string) (inspectResult, error) {
	tables := tableNames
	if len(tables) == 0 {
		q := d.listTablesQuery()
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return inspectResult{}, fmt.Errorf("toolkit/sqltool: list tables: %w", err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return inspectResult{}, fmt.Errorf("toolkit/sqltool: scan table name: %w", err)
			}
			if len(o.allowedTables) == 0 || contains(o.allowedTables, name) {
				tables = append(tables, name)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return inspectResult{}, err
		}
		_ = rows.Close()
	} else if len(o.allowedTables) > 0 {
		filtered := make([]string, 0, len(tables))
		for _, t := range tables {
			if contains(o.allowedTables, t) {
				filtered = append(filtered, t)
			}
		}
		tables = filtered
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
		q, args := d.columnsQuery(table)
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return inspectResult{}, fmt.Errorf("toolkit/sqltool: columns for %s: %w", table, err)
		}
		var rowCount int
		for rows.Next() {
			var name, dataType, nullableStr, defaultVal string
			if driverLower == "sqlite3" || driverLower == "sqlite" {
				var cid, notnull, pk int64
				var dflt sql.NullString
				if err := rows.Scan(&cid, &name, &dataType, &notnull, &dflt, &pk); err != nil {
					_ = rows.Close()
					return inspectResult{}, fmt.Errorf("toolkit/sqltool: scan column: %w", err)
				}
				if notnull == 0 {
					nullableStr = "YES"
				} else {
					nullableStr = "NO"
				}
				if dflt.Valid {
					defaultVal = dflt.String
				}
			} else {
				var def sql.NullString
				if err := rows.Scan(&name, &dataType, &nullableStr, &def); err != nil {
					_ = rows.Close()
					return inspectResult{}, err
				}
				if def.Valid {
					defaultVal = def.String
				}
			}
			if rowCount == 0 {
				fmt.Fprintf(&b, "## Table: %s\n\n| Column | Type | Nullable | Default |\n|--------|------|----------|--------|\n", table)
			}
			rowCount++
			if b.Len() < o.maxSchemaBytes {
				fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", name, dataType, nullableStr, defaultVal)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return inspectResult{}, err
		}
		if rowCount == 0 {
			fmt.Fprintf(&b, "## Table: %s\n\nTable not found or has no columns.\n\n", table)
		} else {
			b.WriteString("\n")
		}
	}
	return inspectResult{Schema: strings.TrimSpace(b.String())}, nil
}

// forbiddenReadOnlyKeywords are SQL keywords that must not appear in execute_read (defense-in-depth).
var forbiddenReadOnlyKeywords = []string{
	"INSERT", "UPDATE", "DELETE", "MERGE", "DROP", "CREATE", "ALTER", "TRUNCATE", "REPLACE",
	"EXEC", "EXECUTE", "CALL", "GRANT", "REVOKE",
}

// validateReadOnlyQuery uses a minimal SQL scanner to reject multi-statement and DML/DDL
// without false positives from string literals or comments (e.g. SELECT 'INSERT' or -- INSERT).
func validateReadOnlyQuery(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return &toolsy.ClientError{Reason: "query is required", Err: toolsy.ErrValidation}
	}
	upper := strings.ToUpper(query)
	firstToken, tokens, err := sqlTokensOutsideStringsAndComments(upper)
	if err != nil {
		return err
	}
	if firstToken != "SELECT" && firstToken != "WITH" {
		return &toolsy.ClientError{Reason: "only SELECT and WITH (CTE) queries are allowed", Err: toolsy.ErrValidation}
	}
	for _, tok := range tokens {
		if slices.Contains(forbiddenReadOnlyKeywords, tok) {
			return &toolsy.ClientError{Reason: "only read-only SELECT queries are allowed", Err: toolsy.ErrValidation}
		}
	}
	return nil
}

// sqlTokensOutsideStringsAndComments returns the first token, all identifier tokens (outside strings/comments),
// or an error if ";" is found outside string/comment. Used for read-only validation.
func sqlTokensOutsideStringsAndComments(upper string) (firstToken string, tokens []string, err error) {
	const (
		stateNormal = iota
		stateSingleQuote
		stateDoubleQuote
		stateLineComment
		stateBlockComment
	)
	var tok strings.Builder
	state := stateNormal
	i := 0
	for i < len(upper) {
		c := upper[i]
		switch state {
		case stateNormal:
			switch {
			case c == '\'':
				state = stateSingleQuote
				i++
			case c == '"':
				state = stateDoubleQuote
				i++
			case c == '-' && i+1 < len(upper) && upper[i+1] == '-':
				state = stateLineComment
				i += 2
			case c == '/' && i+1 < len(upper) && upper[i+1] == '*':
				state = stateBlockComment
				i += 2
			case c == ';':
				return "", nil, &toolsy.ClientError{Reason: "multiple statements not allowed", Err: toolsy.ErrValidation}
			case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_':
				tok.Reset()
				for i < len(upper) {
					cc := upper[i]
					if (cc >= 'A' && cc <= 'Z') || (cc >= '0' && cc <= '9') || cc == '_' {
						tok.WriteByte(cc)
						i++
					} else {
						break
					}
				}
				s := tok.String()
				if s != "" {
					tokens = append(tokens, s)
				}
				continue
			default:
				i++
			}
		case stateSingleQuote:
			if c == '\'' {
				if i+1 < len(upper) && upper[i+1] == '\'' {
					i += 2 // escaped quote
				} else {
					state = stateNormal
					i++
				}
			} else {
				i++
			}
		case stateDoubleQuote:
			if c == '"' {
				if i+1 < len(upper) && upper[i+1] == '"' {
					i += 2
				} else {
					state = stateNormal
					i++
				}
			} else {
				i++
			}
		case stateLineComment:
			if c == '\n' || c == '\r' {
				state = stateNormal
			}
			i++
		case stateBlockComment:
			if c == '*' && i+1 < len(upper) && upper[i+1] == '/' {
				state = stateNormal
				i += 2
			} else {
				i++
			}
		}
	}
	if len(tokens) == 0 {
		return "", tokens, &toolsy.ClientError{Reason: "only SELECT and WITH (CTE) queries are allowed", Err: toolsy.ErrValidation}
	}
	return tokens[0], tokens, nil
}

func doExecuteRead(ctx context.Context, db *sql.DB, o *options, query string) (executeResult, error) {
	query = strings.TrimSpace(query)
	if err := validateReadOnlyQuery(query); err != nil {
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
	// Build Markdown table header (escape pipe and newlines for valid table)
	var b strings.Builder
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
		for i, v := range vals {
			if i > 0 {
				b.WriteString(" | ")
			}
			s := ""
			if v.Valid {
				s = escapeMarkdownCell(truncateCell(v.String, o.maxCellBytes))
			}
			b.WriteString(s)
		}
		b.WriteString("\n")
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return executeResult{}, err
	}
	return executeResult{Result: strings.TrimSpace(b.String()), RowCount: rowCount}, nil
}

func truncateCell(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	need := maxBytes - 3
	if need <= 0 {
		if maxBytes <= 0 {
			return ""
		}
		return s[:maxBytes]
	}
	// Truncate at UTF-8 rune boundary
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > need {
			return s[:n] + "..."
		}
		n += rn
	}
	return s + "..."
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
