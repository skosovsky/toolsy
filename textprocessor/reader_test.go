package textprocessor_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

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
