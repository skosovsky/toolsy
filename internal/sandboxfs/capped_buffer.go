package sandboxfs

import (
	"bytes"

	"github.com/skosovsky/toolsy/textprocessor"
)

// DefaultMaxSandboxOutputBytes is the fail-closed cap for sandbox process stdout/stderr collection.
const DefaultMaxSandboxOutputBytes = 256 * 1024

// DefaultMaxSandboxFileReadBytes is the fail-closed cap for in-sandbox file reads (parity docker archive per-file).
const DefaultMaxSandboxFileReadBytes = 64 * 1024 * 1024

// CappedBuffer accumulates writes up to maxBytes (fail-closed on exceed).
type CappedBuffer struct {
	max      int
	name     string
	buf      bytes.Buffer
	overflow error
}

// NewCappedBuffer returns a buffer that rejects writes beyond maxBytes.
func NewCappedBuffer(name string, maxBytes int) *CappedBuffer {
	return &CappedBuffer{ //nolint:exhaustruct // buf starts empty
		max:  maxBytes,
		name: name,
	}
}

func (c *CappedBuffer) Write(p []byte) (int, error) {
	if c.max > 0 && c.buf.Len()+len(p) > c.max {
		c.overflow = textprocessor.NewReadLimitError(c.name, c.max, textprocessor.ErrReadLimitExceeded)
		return 0, c.overflow
	}
	return c.buf.Write(p)
}

// OverflowErr returns the first fail-closed overflow error, if any.
func (c *CappedBuffer) OverflowErr() error {
	return c.overflow
}

// String returns accumulated bytes as a string.
func (c *CappedBuffer) String() string {
	return c.buf.String()
}
