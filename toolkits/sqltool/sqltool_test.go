package sqltool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/sqlutil"
	"github.com/skosovsky/toolsy/textprocessor"
)

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func decodeSQLChunk[T any](t *testing.T, c toolsy.Chunk) T {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out T
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestInspectSchema_Success(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	inspectTool := tools[0]

	var result InspectResult
	require.NoError(
		t,
		inspectTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[InspectResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Schema, "users")
	require.Contains(t, result.Schema, "id")
	require.Contains(t, result.Schema, "name")
}

func TestInspectSchema_AllowedTables(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE a (x INT)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE TABLE b (y INT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithAllowedTables([]string{"a"}))
	require.NoError(t, err)
	inspectTool := tools[0]

	var result InspectResult
	require.NoError(
		t,
		inspectTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[InspectResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Schema, "## Table: a")
	require.NotContains(t, result.Schema, "## Table: b")
}

func TestInspectSchema_MissingTable(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE users (id INT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	inspectTool := tools[0]

	var result InspectResult
	require.NoError(
		t,
		inspectTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["users","nonexistent_table_xyz"]}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[InspectResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Schema, "## Table: users")
	require.Contains(t, result.Schema, "## Table: nonexistent_table_xyz")
	require.Contains(t, result.Schema, "Table not found or has no columns")
}

func TestExecuteRead_Success(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INT, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t VALUES (1, 'alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[ExecuteResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Result, "alice")
	require.Equal(t, 1, result.RowCount)
}

func TestExecuteRead_MaxRows(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INT)")
	require.NoError(t, err)
	for i := range 200 {
		_, err = db.Exec("INSERT INTO t VALUES (?)", i)
		require.NoError(t, err)
	}

	tools, err := AsTools(db, "sqlite", WithMaxRows(5))
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id FROM t"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[ExecuteResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Result, "[Truncated: max rows reached]")
	require.Equal(t, 5, result.RowCount)
}

func TestExecuteRead_BlocksWrite(t *testing.T) {
	db := openSQLite(t)
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	err = executeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"INSERT INTO t VALUES (1)"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestExecuteRead_StackedStatementsBlocked(t *testing.T) {
	db := openSQLite(t)
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	err = executeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT 1; DELETE FROM t"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "multiple statements")
}

func TestExecuteRead_WritableKeywordBlocked(t *testing.T) {
	db := openSQLite(t)
	_, _ = db.Exec("CREATE TABLE t (id INT)")
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	err = executeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"WITH x AS (DELETE FROM t) SELECT 1"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "read-only")
}

// TestExecuteRead_KeywordInStringAllowed ensures SELECT 'INSERT' is allowed (scanner skips string literals).
func TestExecuteRead_KeywordInStringAllowed(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INT, label TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t VALUES (1, 'INSERT')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	err = executeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, label FROM t WHERE label = 'INSERT'"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSQLChunk[ExecuteResult](t, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.Contains(t, result.Result, "INSERT")
}

// TestExecuteRead_KeywordInCommentAllowed ensures SELECT 1 -- INSERT is allowed (scanner skips comments).
func TestExecuteRead_KeywordInCommentAllowed(t *testing.T) {
	db := openSQLite(t)
	_, _ = db.Exec("CREATE TABLE t (id INT)")
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	err = executeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT 1 AS x -- INSERT here"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSQLChunk[ExecuteResult](t, c)
			return nil
		},
	)
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.RowCount, 1)
}

func TestExecuteRead_MarkdownEscape(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INT, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t VALUES (1, 'a|b')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[ExecuteResult](t, c)
				return nil
			},
		),
	)
	// Pipe in cell should be escaped so table does not break
	require.Contains(t, result.Result, "\\|")
}

func TestAsTools_NilDB(t *testing.T) {
	_, err := AsTools(nil, "sqlite")
	require.Error(t, err)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(openSQLite(t), "sqlite")
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "sql_inspect_schema", tools[0].Manifest().Name)
	require.Equal(t, "sql_execute_read", tools[1].Manifest().Name)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().ReadOnly)
}

func TestAsTools_UnknownDialect(t *testing.T) {
	_, err := AsTools(openSQLite(t), "unknown_driver")
	require.Error(t, err)
}

func TestExecuteRead_MaxCellBytes(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INT, long_text TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t VALUES (1, 'abcdefghijklmnop')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithMaxCellBytes(5))
	require.NoError(t, err)
	executeTool := tools[1]

	var result ExecuteResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, long_text FROM t"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSQLChunk[ExecuteResult](t, c)
				return nil
			},
		),
	)
	// Cell value should be truncated to 5 chars + "..."
	require.Contains(t, result.Result, "...")
	require.NotContains(t, result.Result, "abcdefghijklmnop")
}

func TestValidateReadOnlyQuery_LexicalSubset(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr string
	}{
		{
			name:  "simple select",
			query: "SELECT id FROM t",
		},
		{
			name:  "single quoted string escapes are ignored",
			query: "SELECT 'O''Reilly', label FROM t WHERE label = 'INSERT'",
		},
		{
			name:  "double quoted identifiers are ignored",
			query: `SELECT "колонка", "A""B" FROM "таблица"`,
		},
		{
			name:  "line comment newline resumes scanning",
			query: "SELECT -- INSERT\n id FROM t",
		},
		{
			name:  "block comment is ignored",
			query: "SELECT /* DELETE FROM x */ id FROM t",
		},
		{
			name:  "nested block comments unsupported first closer wins",
			query: "SELECT /* outer /* inner */ FROM t",
		},
		{
			name:  "unicode identifiers outside quotes are ignored by scanner",
			query: "SELECT колонка FROM t",
		},
		{
			name:    "multiple statements blocked",
			query:   "SELECT 1; DELETE FROM t",
			wantErr: "multiple statements",
		},
		{
			name:    "comment only query rejected",
			query:   "/* only comment */",
			wantErr: "only SELECT and WITH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sqlutil.ValidateReadOnlyQuery(tt.query)
			if tt.wantErr != "" {
				require.Error(t, err)
				te, ok := toolsy.AsToolError(err)
				require.True(t, ok)
				assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
				require.Contains(t, te.Reason, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSQLInspect_WithInspectResultFormatter(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithInspectResultFormatter(func(_ InspectResult) (any, error) {
		return map[string]int{"tables": 1}, nil
	}))
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Equal(t, 1, payload["tables"])
}

func TestSQLExecute_WithExecuteResultFormatter(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithExecuteResultFormatter(func(res ExecuteResult) (any, error) {
		return map[string]int{"rows": res.RowCount}, nil
	}))
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Equal(t, 1, payload["rows"])
}

func TestSQLInspect_WithHostResultValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithHostResultValidator(func(v any) error {
		_, ok := v.(InspectResult)
		if !ok {
			return assert.AnError
		}
		return nil
	}))
	require.NoError(t, err)
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestSQLInspect_WithHostResultValidator_Reject(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithHostResultValidator(func(_ any) error {
		return assert.AnError
	}))
	require.NoError(t, err)
	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestSQLExecute_WithHostResultValidator_Reject(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite", WithHostResultValidator(func(_ any) error {
		return assert.AnError
	}))
	require.NoError(t, err)
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestSQLExecute_WithHostResultValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithHostResultValidator(func(v any) error {
			_, ok := v.(ExecuteResult)
			if !ok {
				return assert.AnError
			}
			return nil
		}),
	)
	require.NoError(t, err)
	execTool := tools[1]
	require.NoError(
		t,
		execTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestSQLInspect_FormatterAndValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithInspectResultFormatter(func(_ InspectResult) (any, error) {
			return map[string]string{"kind": "schema"}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["kind"] != "schema" {
				return errors.New("unexpected kind")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]string
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, "schema", payload["kind"])
}

func TestSQLExecute_FormatterAndValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithExecuteResultFormatter(func(res ExecuteResult) (any, error) {
			return map[string]int{"rows": res.RowCount}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]int)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["rows"] < 1 {
				return errors.New("expected rows")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, 1, payload["rows"])
}

func TestSQLInspect_WithMaxSchemaBytes_WithResultFormatter(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithMaxSchemaBytes(80),
		WithInspectResultFormatter(func(_ InspectResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestSQLExecute_WithMaxRows_WithResultFormatter(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithMaxRows(10),
		WithMaxCellBytes(50),
		WithExecuteResultFormatter(func(_ ExecuteResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
	)
	require.NoError(t, err)
	budget := executeWireByteBudget(
		&options{maxRows: 10, maxCellBytes: 50},
		ExecuteResult{Result: "id | name\n--- | ---\n1 | alice"},
	)
	var wire []byte
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), budget+len(textprocessor.TruncationSuffix)+2)
}

func TestInspectSchema_DefaultAndIoCWireSymmetry(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE wide (id INTEGER PRIMARY KEY, payload TEXT)")
	require.NoError(t, err)
	for i := range 40 {
		col := fmt.Sprintf("col_%d_%s", i, strings.Repeat("x", 8))
		_, err = db.Exec("ALTER TABLE wide ADD COLUMN " + col + " TEXT")
		require.NoError(t, err)
	}

	const capBytes = 900
	runInspect := func(tools []toolsy.Tool) []byte {
		t.Helper()
		var wire []byte
		require.NoError(
			t,
			tools[0].Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["wide"]}`)},
				func(c toolsy.Chunk) error {
					wire = append([]byte(nil), c.Data...)
					return nil
				},
			),
		)
		return wire
	}

	defaultTools, err := AsTools(db, "sqlite", WithMaxSchemaBytes(capBytes))
	require.NoError(t, err)
	iocTools, err := AsTools(db, "sqlite",
		WithMaxSchemaBytes(capBytes),
		WithInspectResultFormatter(func(res InspectResult) (any, error) {
			return res, nil
		}),
	)
	require.NoError(t, err)

	defaultWire := runInspect(defaultTools)
	iocWire := runInspect(iocTools)
	require.LessOrEqual(t, len(defaultWire), capBytes+len(textprocessor.TruncationSuffix)+2)
	require.LessOrEqual(t, len(iocWire), capBytes+len(textprocessor.TruncationSuffix)+2)
	require.Equal(t, 1, strings.Count(string(defaultWire), "[Truncated]"))
	require.Equal(t, 1, strings.Count(string(iocWire), "[Truncated]"))
}

func TestSQLInspect_TripleIoC_MaxBytesFormatterValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithMaxSchemaBytes(80),
		WithInspectResultFormatter(func(_ InspectResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok || payload["blob"] == "" {
				return errors.New("invalid payload")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"table_names":["t"]}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestSQLExecute_TripleIoC_MaxRowsFormatterValidator(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO t (name) VALUES ('alice')")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite",
		WithMaxRows(10),
		WithMaxCellBytes(50),
		WithExecuteResultFormatter(func(res ExecuteResult) (any, error) {
			return map[string]int{"rows": res.RowCount}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]int)
			if !ok || payload["rows"] < 1 {
				return errors.New("expected rows")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, name FROM t"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Equal(t, 1, payload["rows"])
}

func TestDoInspectSchema_CancelDuringTables(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tables := []string{"t1", "t2", "t3"}
	_, err := doInspectSchema(ctx, openSQLite(t), "sqlite", &sqliteDialect{}, &options{}, tables)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDoExecuteRead_CancelDuringQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := doExecuteRead(ctx, openSQLite(t), &options{}, "SELECT 1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDoExecuteRead_QueryErrorIsToolError(t *testing.T) {
	db := openSQLite(t)
	_, err := doExecuteRead(context.Background(), db, &options{}, "SELECT id FROM missing_table")
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}
