package sqltool

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestInspectSchema_Success(t *testing.T) {
	db := openSQLite(t)
	_, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	inspectTool := tools[0]

	var result inspectResult
	require.NoError(t, inspectTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(inspectResult); ok {
				result = r
			}
		}
		return nil
	}))
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

	var result inspectResult
	require.NoError(t, inspectTool.Execute(context.Background(), []byte(`{}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(inspectResult); ok {
				result = r
			}
		}
		return nil
	}))
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

	var result inspectResult
	require.NoError(
		t,
		inspectTool.Execute(
			context.Background(),
			[]byte(`{"table_names":["users","nonexistent_table_xyz"]}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(inspectResult); ok {
						result = r
					}
				}
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

	var result executeResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			[]byte(`{"query":"SELECT id, name FROM t"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(executeResult); ok {
						result = r
					}
				}
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

	var result executeResult
	require.NoError(
		t,
		executeTool.Execute(context.Background(), []byte(`{"query":"SELECT id FROM t"}`), func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(executeResult); ok {
					result = r
				}
			}
			return nil
		}),
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
		[]byte(`{"query":"INSERT INTO t VALUES (1)"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestExecuteRead_StackedStatementsBlocked(t *testing.T) {
	db := openSQLite(t)
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	err = executeTool.Execute(
		context.Background(),
		[]byte(`{"query":"SELECT 1; DELETE FROM t"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "multiple statements")
}

func TestExecuteRead_WritableKeywordBlocked(t *testing.T) {
	db := openSQLite(t)
	_, _ = db.Exec("CREATE TABLE t (id INT)")
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	executeTool := tools[1]

	err = executeTool.Execute(
		context.Background(),
		[]byte(`{"query":"WITH x AS (DELETE FROM t) SELECT 1"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "read-only")
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

	var result executeResult
	err = executeTool.Execute(
		context.Background(),
		[]byte(`{"query":"SELECT id, label FROM t WHERE label = 'INSERT'"}`),
		func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(executeResult); ok {
					result = r
				}
			}
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

	var result executeResult
	err = executeTool.Execute(
		context.Background(),
		[]byte(`{"query":"SELECT 1 AS x -- INSERT here"}`),
		func(c toolsy.Chunk) error {
			if c.RawData != nil {
				if r, ok := c.RawData.(executeResult); ok {
					result = r
				}
			}
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

	var result executeResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			[]byte(`{"query":"SELECT id, name FROM t"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(executeResult); ok {
						result = r
					}
				}
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
	require.Contains(t, err.Error(), "nil")
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(openSQLite(t), "sqlite")
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "sql_inspect_schema", tools[0].Name())
	require.Equal(t, "sql_execute_read", tools[1].Name())
}

func TestAsTools_UnknownDialect(t *testing.T) {
	_, err := AsTools(openSQLite(t), "unknown_driver")
	require.Error(t, err)
	require.Contains(t, err.Error(), "toolkit/sqltool")
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

	var result executeResult
	require.NoError(
		t,
		executeTool.Execute(
			context.Background(),
			[]byte(`{"query":"SELECT id, long_text FROM t"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(executeResult); ok {
						result = r
					}
				}
				return nil
			},
		),
	)
	// Cell value should be truncated to 5 chars + "..."
	require.Contains(t, result.Result, "...")
	require.NotContains(t, result.Result, "abcdefghijklmnop")
}
