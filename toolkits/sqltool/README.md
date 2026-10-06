# Toolsy: SQL Toolkit (sqltool)

**Description:** Inspect schema and execute a lexically filtered SELECT/CTE through a host-owned `database/sql` connection. Database privileges define the access boundary. The SELECT lexer is a usability filter, not authorization: SQL functions, dialect syntax and database-side effects require a suitably restricted role.

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
- Execute result: `{"result":"...", "row_count":N, "truncated":false}`. A successful result explicitly reports row or cell display truncation. Row overflow is detected by reading one extra row; no continuation token is invented. Hosts can issue a new query with their own dialect-specific range/offset/keyset contract. This toolkit does not promise stable pagination across changing data.

## Limits and access contract

All zero/negative options select finite defaults; positive values override them. Limits apply to actual database results, even when the query omits a LIMIT clause.

| Option | Default | Enforcement |
| --- | --- | --- |
| `WithMaxRows` | 100 | Returned execute rows; extra row produces explicit truncation |
| `WithMaxColumns` | 128 | Execute columns; inspected columns per table; overflow errors |
| `WithMaxCells` | 10000 | Execute cells; total schema fields (four per column); overflow errors |
| `WithMaxCellBytes` | 200 | Displayed execute value bytes, including Markdown escaping and marker; truncation flagged |
| `WithMaxTables` | 100 | Requested or database-scanned inspect table names, including filtered names; overflow errors |
| `WithMaxSourceBytes` | 4 MiB | Consumed execute names/values before truncation; inspect names/rendered schema accumulation; overflow errors |
| `WithMaxExecuteBytes` | 512 KiB | Complete execute JSON after JSON escaping, including custom formatter output; overflow errors |
| `WithMaxSchemaBytes` | 512 KiB | Complete inspect JSON after JSON escaping, including custom formatter output; overflow errors |

All limit errors return validation errors and no successful partial payload. Source limits bound toolkit consumption and retained data; `database/sql` and the host driver may materialize a complete cell or prefetch rows before a check. Hosts must configure driver/database deadlines, statement/resource limits and field sizes when hard upstream memory bounds are needed. `QueryContext` carries caller cancellation to the driver, and rows are closed on all exits. Driver cancellation behavior is the driver's contract.

`AllowedTables` filters **inspect** only. It is not an execute ACL. The host chooses the connection, read-only role, credentials, dialect and permissions; model queries do not select a new connection. SQL read-only hints do not grant permissions or prove absence of side effects. No query is rewritten with a dialect-dependent LIMIT.

Supported dialects: `postgres`, `pgx`, `mysql`, `sqlite3`, `sqlite`. Output injection: `WithExecuteResultFormatter`, `WithInspectResultFormatter`, `WithHostResultValidator`. Custom formatters run on bounded DTOs and final JSON has the same wire limits; the host remains responsible for allocations inside its callback.

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
