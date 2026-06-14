package textprocessor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

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

// ReadLimited reads up to maxBytes from r with UTF-8 safe truncation; respects ctx cancellation.
func ReadLimited(ctx context.Context, r io.Reader, maxBytes int, suffix string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text, err := ReadAndTruncateValidUTF8(io.LimitReader(r, int64(maxBytes)+1), maxBytes, suffix)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
}

// ReadLimitedBytes reads up to maxBytes from r; returns error when more data is available.
func ReadLimitedBytes(ctx context.Context, r io.Reader, maxBytes int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limited := io.LimitReader(r, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("read exceeds %d bytes", maxBytes)
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

// TruncateStringUTF8NoSuffix truncates s to at most maxBytes at a rune boundary without appending a suffix.
// Use for content pre-caps before wire JSON envelope truncation.
func TruncateStringUTF8NoSuffix(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return truncateStringToBytes(s, maxBytes)
}

// TruncateBytesToValidUTF8String truncates data to maxBytes, normalizes to valid UTF-8, and appends suffix when truncated.
func TruncateBytesToValidUTF8String(data []byte, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(data) <= maxBytes {
		return strings.ToValidUTF8(string(data), "")
	}
	trunc := data[:maxBytes]
	trunc = []byte(strings.ToValidUTF8(string(trunc), ""))
	return string(trunc) + suffix
}

// ReadAndTruncateValidUTF8 reads up to maxBytes+1 from r and returns a valid UTF-8 string with suffix when truncated.
func ReadAndTruncateValidUTF8(r io.Reader, maxBytes int, suffix string) (string, error) {
	if maxBytes <= 0 {
		return "", nil
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
