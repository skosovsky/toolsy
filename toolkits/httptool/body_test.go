package httptool

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestDrainResponseBody_CapsTail(t *testing.T) {
	tail := strings.Repeat("x", DefaultMaxDrainBytes+1024)
	r := strings.NewReader(tail)
	err := DrainResponseBody(context.Background(), r, DefaultMaxDrainBytes)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestDrainResponseBody_ShortBody(t *testing.T) {
	r := strings.NewReader("hello")
	require.NoError(t, DrainResponseBody(context.Background(), r, DefaultMaxDrainBytes))
}

func TestLimitStreamReader_CancelMidRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	type readResult struct {
		err error
	}
	done := make(chan readResult, 1)
	slow := &slowStreamReader{ctx: ctx}
	limited := LimitStreamReaderWithContext(ctx, slow, 1<<20)
	go func() {
		buf := make([]byte, 64)
		_, err := limited.Read(buf)
		done <- readResult{err: err}
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	res := <-done
	require.Error(t, res.err)
	require.ErrorIs(t, res.err, context.Canceled)
}

type slowStreamReader struct {
	ctx context.Context
}

func (r *slowStreamReader) Read(p []byte) (int, error) {
	_ = p
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestLimitStreamReader_CapsTotal(t *testing.T) {
	r := strings.NewReader(strings.Repeat("a", 100))
	limited := LimitStreamReaderWithContext(context.Background(), r, 50)
	buf := make([]byte, 100)
	n, err := limited.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 50, n)
	_, err = limited.Read(buf)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadBodyLimited_ExceedsLimit(t *testing.T) {
	body := strings.Repeat("x", 100)
	data, err := ReadBodyLimited(context.Background(), strings.NewReader(body), 20)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadBodyLimited_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, err := ReadBodyLimited(ctx, strings.NewReader("hello"), 10)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadBodyLimited_ShortBody(t *testing.T) {
	data, err := ReadBodyLimited(context.Background(), strings.NewReader("hello"), 100)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestIsSuccessStatus(t *testing.T) {
	require.True(t, IsSuccessStatus(200))
	require.True(t, IsSuccessStatus(201))
	require.True(t, IsSuccessStatus(204))
	require.True(t, IsSuccessStatus(299))
	require.False(t, IsSuccessStatus(199))
	require.False(t, IsSuccessStatus(300))
	require.False(t, IsSuccessStatus(404))
	require.False(t, IsSuccessStatus(500))
}

type infiniteBodyReader struct{}

func (infiniteBodyReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'z'
	}
	return len(p), nil
}

func TestReadBodyLimited_InfiniteReaderBoundedAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc smoke skipped in -short")
	}
	const limit = 1 << 20
	allocs := testing.AllocsPerRun(3, func() {
		data, err := ReadBodyLimited(context.Background(), infiniteBodyReader{}, limit)
		require.Error(t, err)
		require.Nil(t, data)
		require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	})
	require.Less(t, allocs, float64(64))
}

func TestCloseResponseBody_DrainExceed(t *testing.T) {
	tail := strings.Repeat("x", DefaultMaxDrainBytes+1024)
	body := io.NopCloser(strings.NewReader(tail))
	require.NotPanics(t, func() {
		CloseResponseBody(context.Background(), body)
	})
}

func TestMapReadLimitError(t *testing.T) {
	err := toolsy.MapReadLimitError(textprocessor.ErrReadLimitExceeded, 4096)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "4096")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)

	passThrough := toolsy.MapReadLimitError(io.EOF, 4096)
	require.ErrorIs(t, passThrough, io.EOF)
}

func TestReadBodyLimited_DevZeroFastFinish(t *testing.T) {
	const maxBytes = 1 << 20
	start := time.Now()
	data, err := ReadBodyLimited(context.Background(), devZeroReader{}, maxBytes)
	elapsed := time.Since(start)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestReadBodyLimited_DevZero(t *testing.T) {
	const maxBytes = 1 << 20
	data, err := ReadBodyLimited(context.Background(), devZeroReader{}, maxBytes)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

type devZeroReader struct{}

func (devZeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func devZeroFile() io.Reader {
	f, err := os.Open("/dev/zero")
	if err != nil {
		return devZeroReader{}
	}
	return f
}

func TestReadBodyLimited_DevZeroFile(t *testing.T) {
	const maxBytes = 1 << 20
	r := devZeroFile()
	if f, ok := r.(*os.File); ok {
		defer func() { _ = f.Close() }()
	}
	data, err := ReadBodyLimited(context.Background(), r, maxBytes)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}
