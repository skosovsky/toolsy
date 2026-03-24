package sqltool

import (
	"context"
	"database/sql"
	"errors"
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

const (
	truncationSuffix = "\n[Truncated: max rows reached]"
	// ellipsisSuffixLen is the byte length of "..." appended when truncating cell text.
	ellipsisSuffixLen = 3
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

// forbiddenReadOnlyKeywords returns SQL keywords that must not appear in execute_read (defense-in-depth).
func forbiddenReadOnlyKeywords() []string {
	return []string{
		"INSERT", "UPDATE", "DELETE", "MERGE", "DROP", "CREATE", "ALTER", "TRUNCATE", "REPLACE",
		"EXEC", "EXECUTE", "CALL", "GRANT", "REVOKE",
	}
}

func validationClientError(reason string) *toolsy.ClientError {
	return &toolsy.ClientError{
		Reason:    reason,
		Retryable: false,
		Err:       toolsy.ErrValidation,
	}
}

// validateReadOnlyQuery uses a minimal SQL scanner to reject multi-statement and DML/DDL
// without false positives from string literals or comments (e.g. SELECT 'INSERT' or -- INSERT).
func validateReadOnlyQuery(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return validationClientError("query is required")
	}
	upper := strings.ToUpper(query)
	firstToken, tokens, err := sqlTokensOutsideStringsAndComments(upper)
	if err != nil {
		return err
	}
	if firstToken != "SELECT" && firstToken != "WITH" {
		return validationClientError("only SELECT and WITH (CTE) queries are allowed")
	}
	forbidden := forbiddenReadOnlyKeywords()
	for _, tok := range tokens {
		if slices.Contains(forbidden, tok) {
			return validationClientError("only read-only SELECT queries are allowed")
		}
	}
	return nil
}

// sqlTokensOutsideStringsAndComments returns the first token, all identifier tokens (outside strings/comments),
// or an error if ";" is found outside string/comment. Used for read-only validation.
func sqlTokensOutsideStringsAndComments(upper string) (string, []string, error) {
	s := &upperSQLScan{
		upper:  upper,
		i:      0,
		state:  scanStateNormal,
		tok:    strings.Builder{},
		tokens: nil,
	}
	return s.run()
}

const (
	scanStateNormal = iota
	scanStateSingleQuote
	scanStateDoubleQuote
	scanStateLineComment
	scanStateBlockComment
)

type upperSQLScan struct {
	upper  string
	i      int
	state  int
	tok    strings.Builder
	tokens []string
}

func (s *upperSQLScan) run() (string, []string, error) {
	for s.i < len(s.upper) {
		switch s.state {
		case scanStateNormal:
			if err := s.stepNormal(); err != nil {
				return "", nil, err
			}
		case scanStateSingleQuote:
			s.stepSingleQuote()
		case scanStateDoubleQuote:
			s.stepDoubleQuote()
		case scanStateLineComment:
			s.stepLineComment()
		case scanStateBlockComment:
			s.stepBlockComment()
		}
	}
	if len(s.tokens) == 0 {
		return "", s.tokens, validationClientError("only SELECT and WITH (CTE) queries are allowed")
	}
	return s.tokens[0], s.tokens, nil
}

func (s *upperSQLScan) stepNormal() error {
	c := s.upper[s.i]
	switch {
	case c == '\'':
		s.state = scanStateSingleQuote
		s.i++
	case c == '"':
		s.state = scanStateDoubleQuote
		s.i++
	case c == '-' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '-':
		s.state = scanStateLineComment
		s.i += 2
	case c == '/' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '*':
		s.state = scanStateBlockComment
		s.i += 2
	case c == ';':
		return validationClientError("multiple statements not allowed")
	case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_':
		s.i = s.appendIdentifierToken()
	default:
		s.i++
	}
	return nil
}

func (s *upperSQLScan) appendIdentifierToken() int {
	s.tok.Reset()
	i := s.i
	for i < len(s.upper) {
		cc := s.upper[i]
		if (cc >= 'A' && cc <= 'Z') || (cc >= '0' && cc <= '9') || cc == '_' {
			s.tok.WriteByte(cc)
			i++
		} else {
			break
		}
	}
	if tok := s.tok.String(); tok != "" {
		s.tokens = append(s.tokens, tok)
	}
	return i
}

func (s *upperSQLScan) stepSingleQuote() {
	c := s.upper[s.i]
	if c == '\'' {
		if s.i+1 < len(s.upper) && s.upper[s.i+1] == '\'' {
			s.i += 2 // escaped quote
		} else {
			s.state = scanStateNormal
			s.i++
		}
	} else {
		s.i++
	}
}

func (s *upperSQLScan) stepDoubleQuote() {
	c := s.upper[s.i]
	if c == '"' {
		if s.i+1 < len(s.upper) && s.upper[s.i+1] == '"' {
			s.i += 2
		} else {
			s.state = scanStateNormal
			s.i++
		}
	} else {
		s.i++
	}
}

func (s *upperSQLScan) stepLineComment() {
	c := s.upper[s.i]
	if c == '\n' || c == '\r' {
		s.state = scanStateNormal
	}
	s.i++
}

func (s *upperSQLScan) stepBlockComment() {
	c := s.upper[s.i]
	if c == '*' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '/' {
		s.state = scanStateNormal
		s.i += 2
	} else {
		s.i++
	}
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
			s = escapeMarkdownCell(truncateCell(v.String, maxCellBytes))
		}
		b.WriteString(s)
	}
}

func truncateCell(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	need := maxBytes - ellipsisSuffixLen
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
