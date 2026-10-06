// Package sqltool provides a Text-to-SQL toolkit for agents: inspect database schema
// and execute lexically filtered SELECT queries with finite source, count and wire limits.
// Database role privileges are the authority boundary; the lexer is not authorization.
// AllowedTables restricts inspection only. Driver allocation and cancellation behavior
// remain host responsibilities. Successful display truncation is explicit; no
// pagination token is supplied for arbitrary queries.
//
// ValidateSelectLexicalSubset is an internal lexical usability filter, not a SQL parser.
// The first scanned token must be SELECT or WITH; a fixed set of statement keywords
// and semicolons outside literals/comments reject. It scans ASCII identifier tokens
// after uppercasing, skips single/double quotes with doubled-quote escapes, -- line
// comments and /*...*/ blocks ending at the first closer. Nested comments and dialect
// backticks, brackets, dollar quotes, backslash escapes, # comments and executable
// comments have no corresponding dialect-aware interpretation. Unrecognized text and
// unterminated quotes/comments are left to the database; SQL grammar is not validated.
// SELECT functions and SELECT INTO illustrate why acceptance cannot prove read-only
// behavior. Hosts must restrict database writes and callable routine side effects.
package sqltool
