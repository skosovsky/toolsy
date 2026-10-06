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

Zero limits select finite defaults; negative limits reject construction; positive values override them. Limits apply to actual database results, even when the query omits a LIMIT clause.

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

## Lexical subset and dialect limits

Internally `ValidateSelectLexicalSubset` requires the first scanned token to be
`SELECT` or `WITH`. Outside single/double quotes and comments it rejects every `;`
(including a single trailing terminator) and these whole tokens: `INSERT`, `UPDATE`,
`DELETE`, `MERGE`, `DROP`, `CREATE`, `ALTER`, `TRUNCATE`, `REPLACE`, `EXEC`, `EXECUTE`,
`CALL`, `GRANT`, `REVOKE`. Keyword-like substrings are not blocked (`updated_at`).
It skips doubled single-quoted literals, doubled double-quoted identifiers, `--`
line comments and `/*...*/` blocks ending at their first closer. Tokens are ASCII
identifier characters scanned after uppercasing; this is not a SQL grammar.

The supported driver names select **schema inspection queries**, not dialect-aware
lexers. PostgreSQL dollar quotes/nested comments, MySQL backticks/backslash escapes/
`#` or executable comments, and SQLite bracket/backtick identifiers have no matching
dialect interpretation here. Such syntax may be rejected or accidentally accepted;
there is no dialect compatibility or safety certification. Unknown text and
unterminated literals/comments are left to the database to accept or reject.

Acceptance says neither that SQL is valid nor that it has no effects. For example,
`SELECT host_function()` and `SELECT ... INTO new_table` pass this token filter;
functions and SELECT INTO can have effects under database-specific semantics.
Restrict the database role, callable routines/extensions, filesystem/network access
and connection capabilities in the host. SQLite read-only/query-only connections
reject database writes but do not prevent effects inside host-registered functions.
`sql_execute_read` and its read-only metadata describe the host's intended restricted
connection, rather than a result proved by scanning the query.

## Runnable host example

Run `go run ./examples/host` from this module. The [local SQLite example](examples/host/main.go)
creates a disposable fixture, closes its setup connection, opens the fixture with
`mode=ro`, builds tools with finite budgets and executes a SELECT. A direct INSERT
on the same connection is rejected by the database. The example registers no host
SQL functions, removes its fixture on exit and requires no external service. It
verifies SQLite only; production PostgreSQL/MySQL roles and routine permissions
remain host configuration. `WithAllowedTables` restricts schema inspection only.


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.
