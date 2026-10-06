package sqltool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/sqlutil"
)

func TestSelectLexicalSubsetDoesNotProveValidityOrEffects(t *testing.T) {
	for _, query := range []string{
		"SELECT host_effect()", "SELECT 1 INTO new_table", "WITH anything",
		"SELECT 'unterminated", "SELECT /* unterminated", "SELECT [unterminated",
	} {
		// Arrange: no SQL grammar or function authority check is promised.
		// Act.
		err := sqlutil.ValidateSelectLexicalSubset(query)
		// Assert.
		require.NoError(t, err, query)
	}
	for _, query := range []string{"DELETE FROM t", "SELECT 1;", "WITH x AS (DELETE FROM t) SELECT 1"} {
		// Act.
		err := sqlutil.ValidateSelectLexicalSubset(query)
		// Assert.
		require.ErrorIs(t, err, toolsy.ErrValidation, query)
	}
}

// d34Connector keeps function registration private to one fixture's driver.
type d34Connector struct{ implementation *sqlite.Driver }

func (c d34Connector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.implementation.Open(":memory:")
}
func (c d34Connector) Driver() driver.Driver { return c.implementation }

func TestSelectHostFunctionEffectsRemainHostAuthority(t *testing.T) {
	// Arrange: a host-owned driver registers a function with an observable host effect.
	var effects atomic.Int64
	drv := &sqlite.Driver{}
	require.NoError(t, drv.RegisterScalarFunction("host_effect", 0,
		func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			return effects.Add(1), nil
		}))
	db := sql.OpenDB(d34Connector{implementation: drv})
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	_, err := db.ExecContext(t.Context(), "CREATE TABLE guarded (n INTEGER)")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "PRAGMA query_only=ON")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "INSERT INTO guarded VALUES (1)")
	require.Error(t, err)
	var sqliteErr *sqlite.Error
	require.ErrorAs(t, err, &sqliteErr)
	require.Equal(t, 8, sqliteErr.Code()&0xff, "SQLite SQLITE_READONLY")
	tools, err := AsTools(db, "sqlite")
	require.NoError(t, err)
	// Act: the accepted SELECT calls the registered host function despite query_only.
	var output ExecuteResult
	err = tools[1].Execute(t.Context(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT host_effect() AS effect"}`)},
		func(chunk toolsy.Chunk) error { output = decodeSQLChunk[ExecuteResult](t, chunk); return nil })
	// Assert: database write restriction cannot prevent arbitrary host-function effects.
	require.NoError(t, err)
	require.EqualValues(t, 1, effects.Load())
	require.Equal(t, 1, output.RowCount)
	require.Contains(t, output.Result, "1")
}
