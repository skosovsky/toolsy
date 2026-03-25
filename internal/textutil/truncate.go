package textutil

import (
	"io"
	"strings"
	"unicode/utf8"
)

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
