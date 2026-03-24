package sqltool

import (
	"fmt"
	"strings"
)

// dialect provides driver-specific queries for schema inspection.
type dialect interface {
	listTablesQuery() string
	columnsQuery(tableName string) (query string, args []any)
}

func newDialect(driverName string) (dialect, error) {
	switch strings.ToLower(driverName) {
	case "postgres", "pgx":
		return &postgresDialect{}, nil
	case "mysql":
		return &mysqlDialect{}, nil
	case "sqlite3", "sqlite":
		return &sqliteDialect{}, nil
	default:
		return nil, fmt.Errorf(
			"toolkit/sqltool: unsupported driver %q (use postgres, pgx, mysql, sqlite3, sqlite)",
			driverName,
		)
	}
}

type postgresDialect struct{}

func (postgresDialect) listTablesQuery() string {
	return `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name`
}

func (postgresDialect) columnsQuery(tableName string) (string, []any) {
	return `SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position`, []any{tableName}
}

type mysqlDialect struct{}

func (mysqlDialect) listTablesQuery() string {
	return `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`
}

func (mysqlDialect) columnsQuery(tableName string) (string, []any) {
	return `SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
		ORDER BY ordinal_position`, []any{tableName}
}

type sqliteDialect struct{}

func (sqliteDialect) listTablesQuery() string {
	return `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
}

// SQLite PRAGMA table_info(?) does not support bound parameters; use pragma_table_info(?) function.
func (sqliteDialect) columnsQuery(tableName string) (string, []any) {
	return `SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, []any{tableName}
}
