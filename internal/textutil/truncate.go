package textutil

import (
	"io"

	"github.com/skosovsky/toolsy/textprocessor"
)

// Deprecated: use github.com/skosovsky/toolsy/textprocessor instead.

// TruncateStringUTF8 truncates s to at most maxBytes at a rune boundary and appends suffix.
func TruncateStringUTF8(s string, maxBytes int, suffix string) string {
	return textprocessor.TruncateStringUTF8(s, maxBytes, suffix)
}

// TruncateBytesToValidUTF8String truncates data to maxBytes, normalizes to valid UTF-8, and appends suffix when truncated.
func TruncateBytesToValidUTF8String(data []byte, maxBytes int, suffix string) string {
	return textprocessor.TruncateBytesToValidUTF8String(data, maxBytes, suffix)
}

// ReadAndTruncateValidUTF8 reads up to maxBytes+1 from r and returns a valid UTF-8 string with suffix when truncated.
func ReadAndTruncateValidUTF8(r io.Reader, maxBytes int, suffix string) (string, error) {
	return textprocessor.ReadAndTruncateValidUTF8(r, maxBytes, suffix)
}

// TruncateBytes truncates raw bytes to maxBytes and appends suffix when truncated.
func TruncateBytes(data []byte, maxBytes int, suffix string) []byte {
	return textprocessor.TruncateBytes(data, maxBytes, suffix)
}
