package textprocessor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrReadLimitExceeded is returned when a fail-closed read exceeds the configured byte limit.
// On limit errors callers receive nil data — never partial bytes alongside the error.
var ErrReadLimitExceeded = errors.New("read operation exceeded configured byte limit")

// ReadLimitError carries structured read-limit metadata. Prefer this over regex parsing of Error().
type ReadLimitError struct {
	Subject  string
	MaxBytes int
	Cause    error
}

func (e *ReadLimitError) Error() string {
	cause := e.Cause
	if cause == nil {
		cause = ErrReadLimitExceeded
	}
	if e.Subject != "" && e.MaxBytes > 0 {
		return fmt.Sprintf("%s exceeds %d byte limit: %v", e.Subject, e.MaxBytes, cause)
	}
	if e.MaxBytes > 0 {
		return fmt.Sprintf("exceeds %d byte limit: %v", e.MaxBytes, cause)
	}
	return cause.Error()
}

func (e *ReadLimitError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return ErrReadLimitExceeded
}

// NewReadLimitError builds a typed read-limit error with subject and cap.
func NewReadLimitError(subject string, maxBytes int, cause error) error {
	if cause == nil {
		cause = ErrReadLimitExceeded
	}
	return &ReadLimitError{Subject: subject, MaxBytes: maxBytes, Cause: cause}
}

// IsReadLimitExceeded reports whether err is or wraps ErrReadLimitExceeded.
func IsReadLimitExceeded(err error) bool {
	return errors.Is(err, ErrReadLimitExceeded)
}

// TruncationSuffix is appended when toolkit output is truncated at a byte limit.
const TruncationSuffix = "\n[Truncated]"

// ContractsTruncationSuffix is used by OpenAPI/GraphQL/gRPC contract tools.
const ContractsTruncationSuffix = "\n[Truncated. Use pagination or filters.]"

// SQLRowsTruncationSuffix is appended when sqltool row output hits max rows.
const SQLRowsTruncationSuffix = "\n[Truncated: max rows reached]"

// SQLCellTruncationSuffix is appended when a single sqltool cell is truncated.
const SQLCellTruncationSuffix = "..."

// SearchResultsTruncationSuffix is appended when web search markdown hits maxSearchResultsDisplayed.
const SearchResultsTruncationSuffix = "... [truncated]\n"

// ReadAndTruncate reads up to maxBytes from r with UTF-8 safe truncation and suffix (explicit opt-in).
// Use for LLM/display tiers only — not for transport or security-sensitive reads.
func ReadAndTruncate(ctx context.Context, r io.Reader, maxBytes int, suffix string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text, err := ReadAndTruncateValidUTF8(
		ReaderWithContext(ctx, io.LimitReader(r, int64(maxBytes)+1)),
		maxBytes,
		suffix,
	)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
}

// ReadLimitedBytes reads at most maxBytes from r (fail-closed).
// If more data is available, returns nil, ErrReadLimitExceeded.
// When maxBytes <= 0, any non-empty input is treated as exceeding the limit.
func ReadLimitedBytes(ctx context.Context, r io.Reader, maxBytes int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limited := io.LimitReader(r, int64(maxBytes)+1)
	data, err := io.ReadAll(ReaderWithContext(ctx, limited))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, ErrReadLimitExceeded
	}
	return data, nil
}

// TruncateStringUTF8 truncates s to at most maxBytes at a rune boundary and appends suffix.
func TruncateStringUTF8(s string, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	suffix = truncateStringToBytes(suffix, maxBytes)
	need := maxBytes - len(suffix)
	if need <= 0 {
		return suffix
	}
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > need {
			return s[:n] + suffix
		}
		n += rn
	}
	return s
}

func truncateStringToBytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > maxBytes {
			return s[:n]
		}
		n += rn
	}
	return s
}

// TruncateBytesToValidUTF8String truncates data to maxBytes at a UTF-8 rune boundary and appends suffix when truncated.
// Suffix is appended after the prefix cap (display/wire tier); total length may exceed maxBytes.
func TruncateBytesToValidUTF8String(data []byte, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	s := strings.ToValidUTF8(string(data), "")
	if len(s) <= maxBytes {
		return s
	}
	return truncateStringToBytes(s, maxBytes) + suffix
}

// ReadAndTruncateValidUTF8 reads up to maxBytes+1 from r and returns a UTF-8 string truncated at a rune boundary.
// Display-only helper without context cancellation; prefer ReadAndTruncate(ctx, r, maxBytes, suffix) for I/O paths.
func ReadAndTruncateValidUTF8(r io.Reader, maxBytes int, suffix string) (string, error) {
	if maxBytes <= 0 {
		return "", errors.New("maxBytes must be positive")
	}
	limited := io.LimitReader(r, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	return TruncateBytesToValidUTF8String(data, maxBytes, suffix), nil
}

// TruncateBytes truncates raw bytes to maxBytes and appends suffix when truncated.
func TruncateBytes(data []byte, maxBytes int, suffix string) []byte {
	if maxBytes <= 0 {
		return []byte{}
	}
	if len(data) <= maxBytes {
		return data
	}
	out := make([]byte, maxBytes, maxBytes+len(suffix))
	copy(out, data[:maxBytes])
	out = append(out, suffix...)
	return out
}

// TruncateBytesByRunes truncates UTF-8 data to maxRunes runes and appends suffix.
func TruncateBytesByRunes(data []byte, maxRunes int, suffix string) []byte {
	if maxRunes <= 0 {
		return nil
	}
	contentRunes := []rune(string(data))
	if len(contentRunes) <= maxRunes {
		out := make([]byte, len(data))
		copy(out, data)
		return out
	}
	if suffix == "" {
		return []byte(string(contentRunes[:maxRunes]))
	}
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= maxRunes {
		return []byte(string(suffixRunes[:maxRunes]))
	}
	prefixRunes := maxRunes - len(suffixRunes)
	return []byte(string(contentRunes[:prefixRunes]) + suffix)
}

var (
	readLimitSubjectLinePattern = regexp.MustCompile(`: (.+?) exceeds (\d+) byte limit`)
	readLimitBytePattern        = regexp.MustCompile(`exceeds (\d+) byte limit`)
)

const (
	readLimitSubjectMatchIndex  = 1
	readLimitMaxBytesMatchIndex = 2
	readLimitMinSubmatchCount   = 3
)

// ReadLimitSubject extracts the capped stream or domain name from a read-limit error chain.
func ReadLimitSubject(err error) string {
	var rl *ReadLimitError
	if errors.As(err, &rl) && strings.TrimSpace(rl.Subject) != "" {
		return strings.TrimSpace(rl.Subject)
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		matches := readLimitSubjectLinePattern.FindStringSubmatch(e.Error())
		if len(matches) >= readLimitSubjectMatchIndex+1 {
			return strings.TrimSpace(matches[readLimitSubjectMatchIndex])
		}
	}
	return ""
}

// ReadLimitMaxBytes extracts the byte cap from a read-limit error chain.
func ReadLimitMaxBytes(err error) int {
	var rl *ReadLimitError
	if errors.As(err, &rl) && rl.MaxBytes > 0 {
		return rl.MaxBytes
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		matches := readLimitSubjectLinePattern.FindStringSubmatch(e.Error())
		if len(matches) >= readLimitMinSubmatchCount {
			var n int
			_, scanErr := fmt.Sscanf(matches[readLimitMaxBytesMatchIndex], "%d", &n)
			if scanErr == nil && n > 0 {
				return n
			}
		}
		matches = readLimitBytePattern.FindStringSubmatch(e.Error())
		if len(matches) >= 2 {
			var n int
			_, scanErr := fmt.Sscanf(matches[1], "%d", &n)
			if scanErr == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}
