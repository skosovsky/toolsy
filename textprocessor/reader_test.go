package textprocessor_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

type gatedReader struct {
	started chan struct{}
	release chan struct{}
}

func (r *gatedReader) Read(p []byte) (int, error) {
	close(r.started)
	<-r.release
	p[0] = 'a'
	return 1, nil
}

func TestReaderWithContext_CancellationIsBetweenReads(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	underlying := &gatedReader{started: make(chan struct{}), release: make(chan struct{})}
	reader := textprocessor.ReaderWithContext(ctx, underlying)
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := reader.Read(make([]byte, 1))
		done <- result{n: n, err: err}
	}()
	<-underlying.started

	cancel()
	select {
	case <-done:
		t.Fatal("generic reader unexpectedly interrupted a blocked read")
	default:
	}
	close(underlying.release)
	first := <-done
	n, err := reader.Read(make([]byte, 1))

	require.NoError(t, first.err)
	require.Equal(t, 1, first.n)
	require.Zero(t, n)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReaderWithContext_NilContextPreservesReader(t *testing.T) {
	t.Parallel()
	reader := io.LimitReader(&slowByteReader{ctx: context.Background()}, 1)

	//nolint:staticcheck // Explicitly verify the documented nil-context passthrough.
	wrapped := textprocessor.ReaderWithContext(nil, reader)

	require.Same(t, reader, wrapped)
}

type slowByteReader struct {
	ctx context.Context
}

func (r *slowByteReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	time.Sleep(5 * time.Millisecond)
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = 'a'
	return 1, nil
}

func TestReadLimitedBytes_CancelMidRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		data, err := textprocessor.ReadLimitedBytes(ctx, &slowByteReader{ctx: ctx}, 1<<20)
		done <- readResult{data: data, err: err}
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	res := <-done
	require.Error(t, res.err)
	require.Nil(t, res.data)
	require.ErrorIs(t, res.err, context.Canceled)
}
