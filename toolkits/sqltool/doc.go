// Package sqltool provides a Text-to-SQL toolkit for agents: inspect database schema
// and execute read-only SELECT queries with row and cell size limits.
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
