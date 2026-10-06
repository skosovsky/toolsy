package textprocessor

import (
	"context"
	"io"
)

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ReaderWithContext checks ctx before each Read. Cancellation is cooperative:
// it cannot interrupt a Read already blocked inside r. The owner of a transport
// must close its body/process or set deadlines to unblock ongoing I/O.
// It neither owns nor closes r and creates no goroutines. A nil ctx returns r.
func ReaderWithContext(ctx context.Context, r io.Reader) io.Reader {
	if ctx == nil {
		return r
	}
	return &ctxReader{ctx: ctx, r: r}
}
