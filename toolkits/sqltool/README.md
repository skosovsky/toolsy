# Toolsy: SQL Toolkit (sqltool)

**Description:** Lets the agent inspect database schema and run read-only SELECT (and WITH/CTE) queries. Results are returned as Markdown tables with row and cell size limits to protect context window and memory.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/sqltool
```

**Dependencies:** stdlib `database/sql`; requires `github.com/skosovsky/toolsy` (core).

## Available tools

| Tool                 | Description                      | Input                                      |
| -------------------- | -------------------------------- | ------------------------------------------ |
| `sql_inspect_schema` | Get DDL/schema of allowed tables | `{}` or `{"table_names": ["string", ...]}` |
| `sql_execute_read`   | Execute a SELECT query           | `{"query": "string"}`                      |

- Inspect result: `{"schema": "## Table: t\n\n| Column | Type | ..."}` (Markdown).
- Execute result: `{"result": "col1 | col2\n---\n...", "row_count": N}`. Rows are capped by `MaxRows`; cell values are truncated by `MaxCellBytes`. If the limit is reached, `[Truncated: max rows reached]` is appended.

## Configuration & Security

> **Warning:** Use a **read-only database user** for the connection passed to `AsTools`. The toolkit rejects multiple statements (`;`), DML/DDL keywords in the query body, and returns an error if `db` is nil.

- **MaxRows:** Use `WithMaxRows(n)` to cap returned rows (default 100).
- **MaxCellBytes:** Use `WithMaxCellBytes(n)` to truncate long cell values and avoid context-window blowup (default 200).
- **MaxSchemaBytes:** Use `WithMaxSchemaBytes(n)` to cap **inspect** wire JSON (default 512 KiB). Wire truncation uses `textprocessor.TruncationSuffix` once on final JSON via `format.CapWireJSON`.
- **Execute caps:** `WithMaxRows` / `WithMaxCellBytes` are **semantic** limits on query result markdown (row/cell suffixes), not a wire byte budget. There is no `WithMaxExecuteBytes`; execute formatter output is not wire-capped unless the host formatter returns a smaller payload.
- **AllowedTables:** Use `WithAllowedTables([]string{"t1","t2"})` to restrict schema inspection to specific tables.
- **Dialects:** Supported drivers: `postgres`, `pgx`, `mysql`, `sqlite3`, `sqlite`.
- **IoC:** `WithExecuteResultFormatter`, `WithInspectResultFormatter`, and `WithHostResultValidator` (inspect and execute tools).

## Quick start

```go
package main

import (
	"database/sql"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/sqltool"
)

func main() {
	db, err := sql.Open("postgres", "postgres://readonly:...@localhost/db?sslmode=disable")
	if err != nil {
		panic(err)
	}
	builder := toolsy.NewRegistryBuilder()

	tools, err := sqltool.AsTools(db, "postgres", sqltool.WithMaxRows(50))
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```
