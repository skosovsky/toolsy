package httptool

import (
	"context"
	"io"

	"github.com/skosovsky/toolsy/textprocessor"
)

// DefaultMaxDrainBytes is the cap for draining unread HTTP response body tails (keep-alive reuse).
const DefaultMaxDrainBytes = 64 * 1024

// DrainResponseBody reads up to maxBytes from r so the connection can be reused.
// Cancellation is checked before and during the read via textprocessor.ReaderWithContext.
func DrainResponseBody(ctx context.Context, r io.Reader, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDrainBytes
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	limited := io.LimitReader(r, int64(maxBytes)+1)
	n, err := io.Copy(io.Discard, textprocessor.ReaderWithContext(ctx, limited))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if n > int64(maxBytes) {
		return textprocessor.ErrReadLimitExceeded
	}
	return nil
}

// DefaultMaxSSEStreamBytes is the default total byte budget for long-lived SSE/stdio JSON-RPC streams.
const DefaultMaxSSEStreamBytes = 16 * 1024 * 1024

// limitedStreamReader enforces a byte budget on long-lived streams (SSE, stdio JSON-RPC).
// Unlike ReadLimitedBytes, Read may return partial data together with ErrReadLimitExceeded.
type limitedStreamReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (l *limitedStreamReader) Read(p []byte) (int, error) {
	if l.n >= l.max {
		return 0, textprocessor.ErrReadLimitExceeded
	}
	remaining := l.max - l.n
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		return n, textprocessor.ErrReadLimitExceeded
	}
	return n, err
}

// LimitStreamReaderWithContext wraps r with a byte budget and honors ctx cancellation during Read.
func LimitStreamReaderWithContext(ctx context.Context, r io.Reader, maxBytes int) io.Reader {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSSEStreamBytes
	}
	return &limitedStreamReader{r: textprocessor.ReaderWithContext(ctx, r), n: 0, max: int64(maxBytes)}
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

// LimitStreamReadCloserWithContext wraps r with LimitStreamReaderWithContext and preserves Close.
func LimitStreamReadCloserWithContext(ctx context.Context, r io.ReadCloser, maxBytes int) io.ReadCloser {
	if r == nil {
		return nil
	}
	return limitedReadCloser{Reader: LimitStreamReaderWithContext(ctx, r, maxBytes), closer: r}
}

// CloseResponseBody drains up to DefaultMaxDrainBytes from body so the connection can be reused, then closes it.
// Drain errors (including ErrReadLimitExceeded when the unread tail exceeds the drain cap) are ignored because
// the caller has already consumed the response; use DrainResponseBody when the drain result matters.
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

// ReadBodyLimited reads at most maxBytes from r (fail-closed).
// Returns nil, textprocessor.ErrReadLimitExceeded when more data is available.
func ReadBodyLimited(ctx context.Context, r io.Reader, maxBytes int) ([]byte, error) {
	return textprocessor.ReadLimitedBytes(ctx, r, maxBytes)
}
