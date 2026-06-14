package httptool

import (
	"context"
	"fmt"
	"io"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

const truncationSuffix = textprocessor.TruncationSuffix

// DefaultMaxDrainBytes is the cap for draining unread HTTP response body tails (keep-alive reuse).
const DefaultMaxDrainBytes = 64 * 1024

// DrainResponseBody reads up to maxBytes from r so the connection can be reused; respects ctx cancellation.
func DrainResponseBody(ctx context.Context, r io.Reader, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDrainBytes
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	limited := io.LimitReader(r, int64(maxBytes)+1)
	n, err := io.Copy(io.Discard, limited)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if n > int64(maxBytes) {
		return fmt.Errorf("toolkit/httptool: response drain exceeds %d bytes", maxBytes)
	}
	return nil
}

// DefaultMaxSSEStreamBytes is the default total byte budget for long-lived SSE/stdio JSON-RPC streams.
const DefaultMaxSSEStreamBytes = 16 * 1024 * 1024

type limitedStreamReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (l *limitedStreamReader) Read(p []byte) (int, error) {
	if l.n >= l.max {
		return 0, fmt.Errorf("toolkit/httptool: stream exceeds %d bytes", l.max)
	}
	remaining := l.max - l.n
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		return n, fmt.Errorf("toolkit/httptool: stream exceeds %d bytes", l.max)
	}
	return n, err
}

// LimitStreamReader wraps r with a total byte budget for long-lived streams (SSE, stdio JSON-RPC).
func LimitStreamReader(r io.Reader, maxBytes int) io.Reader {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSSEStreamBytes
	}
	return &limitedStreamReader{r: r, n: 0, max: int64(maxBytes)}
}

type limitedReadCloser struct {
	io.Reader

	closer io.Closer
}

func (l limitedReadCloser) Close() error {
	if l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

// LimitStreamReadCloser wraps r with LimitStreamReader and preserves Close.
func LimitStreamReadCloser(r io.ReadCloser, maxBytes int) io.ReadCloser {
	if r == nil {
		return nil
	}
	return limitedReadCloser{Reader: LimitStreamReader(r, maxBytes), closer: r}
}

// CloseResponseBody drains up to DefaultMaxDrainBytes then closes the body.
func CloseResponseBody(ctx context.Context, body io.ReadCloser) {
	if body == nil {
		return
	}
	_ = DrainResponseBody(ctx, body, DefaultMaxDrainBytes)
	_ = body.Close()
}

// IsSuccessStatus reports whether code is an HTTP 2xx success status.
func IsSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}

// ReadBodyLimited reads up to maxBytes from r with UTF-8 safe truncation and [textprocessor.TruncationSuffix].
// Respects ctx cancellation before and after read.
func ReadBodyLimited(ctx context.Context, r io.Reader, maxBytes int) (string, error) {
	text, err := textprocessor.ReadLimited(ctx, r, maxBytes, textprocessor.TruncationSuffix)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: context: %w", ctxErr))
		}
		return "", toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: read body: %w", err))
	}
	return text, nil
}
