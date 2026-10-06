// This host owns its SQLite fixture and enforces database writes with mode=ro.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"modernc.org/sqlite"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/sqltool"
)

func main() {
	if err := run(context.Background()); err != nil {
		panic(err)
	}
}

func run(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "toolsy-sql-host-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	databaseURL := url.URL{Scheme: "file", Path: filepath.Join(dir, "fixture.db")}
	if seedErr := seed(ctx, databaseURL.String()); seedErr != nil {
		return seedErr
	}
	databaseURL.RawQuery = "mode=ro"
	connector, err := sqlite.NewConnector(databaseURL.String())
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	tools, err := sqltool.AsTools(
		db,
		"sqlite",
		sqltool.WithMaxRows(2),
		sqltool.WithAllowedTables([]string{"public_rows"}),
	)
	if err != nil {
		return err
	}
	if err := tools[1].Execute(ctx, toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"SELECT id, label FROM public_rows ORDER BY id"}`)},
		func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil },
	); err != nil {
		return err
	}
	// The database, rather than the lexical toolkit filter, enforces this write denial.
	_, writeErr := db.ExecContext(ctx, "INSERT INTO public_rows VALUES (2, 'denied')")
	if writeErr == nil {
		return errors.New("host example: read-only database unexpectedly allowed INSERT")
	}
	var sqliteErr *sqlite.Error
	// SQLite primary result code 8 is SQLITE_READONLY; extended codes keep its low byte.
	if !errors.As(writeErr, &sqliteErr) || sqliteErr.Code()&0xff != 8 {
		return fmt.Errorf("host example: expected SQLite read-only denial: %w", writeErr)
	}
	fmt.Println("database rejects INSERT")
	return nil
}

func seed(ctx context.Context, dsn string) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, createErr := db.ExecContext(ctx, "CREATE TABLE public_rows (id INTEGER, label TEXT)"); createErr != nil {
		return createErr
	}
	_, err = db.ExecContext(ctx, "INSERT INTO public_rows VALUES (1, 'local')")
	return err
}
