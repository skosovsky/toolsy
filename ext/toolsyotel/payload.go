package toolsyotel

import (
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

const payloadTruncatedSuffix = "... [truncated]"

// truncatePayload limits s to at most limit bytes of content; when truncated, appends payloadTruncatedSuffix.
func truncatePayload(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	prefix := utf8SafePrefix(s, limit)
	return prefix + payloadTruncatedSuffix
}

func utf8SafePrefix(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// payloadAccumulator collects streamed output up to maxSize bytes, then appends the truncation suffix once.
type payloadAccumulator struct {
	mu        sync.Mutex
	builder   strings.Builder
	maxSize   int
	truncated bool
}

func newPayloadAccumulator(maxSize int) *payloadAccumulator {
	a := &payloadAccumulator{
		mu:        sync.Mutex{},
		builder:   strings.Builder{},
		maxSize:   maxSize,
		truncated: false,
	}
	a.builder.Grow(maxSize + len(payloadTruncatedSuffix))
	return a
}

func (a *payloadAccumulator) append(part string) {
	if part == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.truncated {
		return
	}
	remaining := a.maxSize - a.builder.Len()
	if remaining <= 0 {
		a.writeSuffixLocked()
		return
	}
	if len(part) <= remaining {
		a.builder.WriteString(part)
		return
	}
	a.builder.WriteString(utf8SafePrefix(part, remaining))
	a.writeSuffixLocked()
}

func (a *payloadAccumulator) writeSuffixLocked() {
	if a.truncated {
		return
	}
	a.truncated = true
	a.builder.WriteString(payloadTruncatedSuffix)
}

func (a *payloadAccumulator) String() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.builder.String()
}

func chunkPayloadText(c toolsy.Chunk) string {
	if len(c.Data) == 0 {
		return ""
	}
	if !utf8.Valid(c.Data) {
		return ""
	}
	return string(c.Data)
}
