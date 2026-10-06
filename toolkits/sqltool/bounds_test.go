package sqltool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestExecuteActualResultBounds(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		option      Option
	}{
		{"columns", "SELECT 1 AS a, 2 AS b", WithMaxColumns(1)},
		{"cells", "SELECT 1 AS a, 2 AS b UNION ALL SELECT 3, 4", WithMaxCells(3)},
		{"source before truncation", "SELECT 'abcdefghijk' AS a", WithMaxSourceBytes(10)},
		{"escaped wire", "SELECT '<><><><><><><>' AS a", WithMaxExecuteBytes(80)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			tools, err := AsTools(openSQLite(t), "sqlite", tc.option)
			require.NoError(t, err)
			args, err := json.Marshal(executeArgs{Query: tc.query})
			require.NoError(t, err)
			calls := 0
			// Act.
			err = tools[1].Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: args},
				func(toolsy.Chunk) error { calls++; return nil },
			)
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls)
		})
	}
}

func TestExecuteFormatterExactWireBudget(t *testing.T) {
	// Arrange.
	tools, err := AsTools(
		openSQLite(t),
		"sqlite",
		WithMaxExecuteBytes(100),
		WithExecuteResultFormatter(func(ExecuteResult) (any, error) { return strings.Repeat("<", 30), nil }),
	)
	require.NoError(t, err)
	calls := 0
	// Act.
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT 1 AS a"}`)},
		func(toolsy.Chunk) error { calls++; return nil },
	)
	// Assert.
	require.Error(t, err)
	require.Zero(t, calls)
}

func TestBoundedCellIncludesEscapingMarkerUTF8(t *testing.T) {
	for _, value := range []string{strings.Repeat("|", 20), strings.Repeat("界", 20), string([]byte{0xff, 0xfe, 0xfd})} {
		for limit := 1; limit < 20; limit++ {
			// Arrange / Act.
			cell := boundedCell(value, limit)
			// Assert.
			require.LessOrEqual(t, len(cell), limit)
			require.True(t, json.Valid([]byte(`"`+strings.ReplaceAll(cell, `\`, `\\`)+`"`)))
		}
	}
}

func TestInspectActualResultBounds(t *testing.T) {
	for _, option := range []Option{WithMaxTables(1), WithMaxColumns(1), WithMaxCells(3), WithMaxSourceBytes(10)} {
		// Arrange.
		db := openSQLite(t)
		_, err := db.Exec("CREATE TABLE one (a TEXT,b TEXT); CREATE TABLE two (a TEXT)")
		require.NoError(t, err)
		tools, err := AsTools(db, "sqlite", option)
		require.NoError(t, err)
		calls := 0
		// Act.
		err = tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(toolsy.Chunk) error { calls++; return nil },
		)
		// Assert.
		require.Error(t, err)
		require.Zero(t, calls)
	}
}

func TestNonpositiveLimitsAreFinite(t *testing.T) {
	// Arrange.
	var o options
	// Act.
	applyDefaults(&o)
	// Assert.
	for _, limit := range []int{o.maxRows, o.maxColumns, o.maxCells, o.maxTables, o.maxCellBytes, o.maxSourceBytes, o.maxExecuteBytes, o.maxSchemaBytes} {
		require.Positive(t, limit)
	}
}

func TestExecuteEscapedCellTruncationIsExplicit(t *testing.T) {
	// Arrange.
	tools, err := AsTools(openSQLite(t), "sqlite", WithMaxCellBytes(5))
	require.NoError(t, err)
	var result ExecuteResult
	// Act.
	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT '||||' AS a"}`)},
		func(c toolsy.Chunk) error { result = decodeSQLChunk[ExecuteResult](t, c); return nil },
	)
	// Assert.
	require.NoError(t, err)
	require.True(t, result.Truncated)
	require.Contains(t, result.Result, "...")
}
