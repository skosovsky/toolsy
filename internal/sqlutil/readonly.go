package sqlutil

import (
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
)

const (
	scanStateNormal = iota
	scanStateSingleQuote
	scanStateDoubleQuote
	scanStateLineComment
	scanStateBlockComment
)

// ValidateReadOnlyQuery rejects multi-statement and mutating SQL using a small lexical subset.
// It supports doubled single-quote literals, doubled double-quoted identifiers, line comments,
// block comments, ASCII unquoted identifiers, and intentionally does not support nested block comments.
func ValidateReadOnlyQuery(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return validationClientError("query is required")
	}
	upper := strings.ToUpper(query)
	firstToken, tokens, err := sqlTokensOutsideStringsAndComments(upper)
	if err != nil {
		return err
	}
	if firstToken != "SELECT" && firstToken != "WITH" {
		return validationClientError("only SELECT and WITH (CTE) queries are allowed")
	}
	forbidden := forbiddenReadOnlyKeywords()
	for _, tok := range tokens {
		if slices.Contains(forbidden, tok) {
			return validationClientError("only read-only SELECT queries are allowed")
		}
	}
	return nil
}

func validationClientError(reason string) *toolsy.ClientError {
	return &toolsy.ClientError{
		Reason:    reason,
		Retryable: false,
		Err:       toolsy.ErrValidation,
	}
}

func forbiddenReadOnlyKeywords() []string {
	return []string{
		"INSERT", "UPDATE", "DELETE", "MERGE", "DROP", "CREATE", "ALTER", "TRUNCATE", "REPLACE",
		"EXEC", "EXECUTE", "CALL", "GRANT", "REVOKE",
	}
}

func sqlTokensOutsideStringsAndComments(upper string) (string, []string, error) {
	s := &upperSQLScan{
		upper:  upper,
		i:      0,
		state:  scanStateNormal,
		tok:    strings.Builder{},
		tokens: nil,
	}
	return s.run()
}

type upperSQLScan struct {
	upper  string
	i      int
	state  int
	tok    strings.Builder
	tokens []string
}

func (s *upperSQLScan) run() (string, []string, error) {
	for s.i < len(s.upper) {
		switch s.state {
		case scanStateNormal:
			if err := s.stepNormal(); err != nil {
				return "", nil, err
			}
		case scanStateSingleQuote:
			s.stepSingleQuote()
		case scanStateDoubleQuote:
			s.stepDoubleQuote()
		case scanStateLineComment:
			s.stepLineComment()
		case scanStateBlockComment:
			s.stepBlockComment()
		}
	}
	if len(s.tokens) == 0 {
		return "", s.tokens, validationClientError("only SELECT and WITH (CTE) queries are allowed")
	}
	return s.tokens[0], s.tokens, nil
}

func (s *upperSQLScan) stepNormal() error {
	c := s.upper[s.i]
	switch {
	case c == '\'':
		s.state = scanStateSingleQuote
		s.i++
	case c == '"':
		s.state = scanStateDoubleQuote
		s.i++
	case c == '-' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '-':
		s.state = scanStateLineComment
		s.i += 2
	case c == '/' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '*':
		s.state = scanStateBlockComment
		s.i += 2
	case c == ';':
		return validationClientError("multiple statements are not allowed")
	case (c >= 'A' && c <= 'Z') || c == '_':
		s.i += s.appendIdentifierToken()
	default:
		s.i++
	}
	return nil
}

func (s *upperSQLScan) appendIdentifierToken() int {
	s.tok.Reset()
	j := s.i
	for j < len(s.upper) {
		c := s.upper[j]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			s.tok.WriteByte(c)
			j++
			continue
		}
		break
	}
	s.tokens = append(s.tokens, s.tok.String())
	return j - s.i
}

func (s *upperSQLScan) stepSingleQuote() {
	if s.upper[s.i] == '\'' {
		if s.i+1 < len(s.upper) && s.upper[s.i+1] == '\'' {
			s.i += 2
			return
		}
		s.state = scanStateNormal
	}
	s.i++
}

func (s *upperSQLScan) stepDoubleQuote() {
	if s.upper[s.i] == '"' {
		if s.i+1 < len(s.upper) && s.upper[s.i+1] == '"' {
			s.i += 2
			return
		}
		s.state = scanStateNormal
	}
	s.i++
}

func (s *upperSQLScan) stepLineComment() {
	if s.upper[s.i] == '\n' {
		s.state = scanStateNormal
	}
	s.i++
}

func (s *upperSQLScan) stepBlockComment() {
	if s.upper[s.i] == '*' && s.i+1 < len(s.upper) && s.upper[s.i+1] == '/' {
		s.state = scanStateNormal
		s.i += 2
		return
	}
	s.i++
}
