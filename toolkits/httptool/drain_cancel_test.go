package httptool_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

type slowDrainReader struct {
	ctx context.Context
}

func (r *slowDrainReader) Read(p []byte) (int, error) {
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
	p[0] = 'b'
	return 1, nil
}

func TestDrainResponseBody_CancelMidDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- httptool.DrainResponseBody(ctx, &slowDrainReader{ctx: ctx}, httptool.DefaultMaxDrainBytes)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	err := <-done
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
