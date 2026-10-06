// Package sqltool provides a Text-to-SQL toolkit for agents: inspect database schema
// and execute lexically filtered SELECT queries with finite source, count and wire limits.
// Database role privileges are the authority boundary; the lexer is not authorization.
// AllowedTables restricts inspection only. Driver allocation and cancellation behavior
// remain host responsibilities. Successful display truncation is explicit; no
// pagination token is supplied for arbitrary queries.
//
// The read-only validator uses a small lexical subset rather than a full SQL parser.
// It supports:
//   - single-quoted literals with doubled quote escaping ('O”Reilly');
//   - double-quoted identifiers with doubled quote escaping ("A""B");
//   - line comments started by --;
//   - block comments delimited by /* and */;
//   - ASCII unquoted identifiers [A-Z0-9_];
//   - rejection of ; outside literals/comments.
//
// Nested block comments are intentionally unsupported. Unicode identifiers must be
// double-quoted; unquoted non-ASCII text is ignored by the scanner and left to the
// database engine to reject when invalid.
package sqltool
