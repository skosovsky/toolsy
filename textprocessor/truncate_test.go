package textprocessor_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestTruncateStringUTF8(t *testing.T) {
	got := textprocessor.TruncateStringUTF8("приветмир", 9, "...")
	require.True(t, utf8.ValidString(got))
	assert.Equal(t, "при...", got)
}

func TestTruncateBytesByRunes(t *testing.T) {
	got := textprocessor.TruncateBytesByRunes([]byte("abcdefghijklmnopqrstuvwxyz"), 12, "...")
	assert.Equal(t, "abcdefghi...", string(got))
}

func TestTruncateBytesToValidUTF8String(t *testing.T) {
	got := textprocessor.TruncateBytesToValidUTF8String([]byte("hello world"), 5, "...")
	assert.Equal(t, "hello...", got)
}

func TestTruncateBytesToValidUTF8String_MultibyteUTF8(t *testing.T) {
	got := textprocessor.TruncateBytesToValidUTF8String([]byte("привет мир"), 9, "...")
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, "прив...", got)
}

func TestReadAndTruncateValidUTF8_NonPositiveLimitWithData(t *testing.T) {
	got, err := textprocessor.ReadAndTruncateValidUTF8(strings.NewReader("hello"), 0, "...")
	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "maxBytes must be positive")
}

func TestReadLimitedBytes_WithinLimit(t *testing.T) {
	data, err := textprocessor.ReadLimitedBytes(context.Background(), strings.NewReader("hello"), 10)
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestReadLimitedBytes_ExceedsLimit(t *testing.T) {
	body := strings.Repeat("x", 100)
	data, err := textprocessor.ReadLimitedBytes(context.Background(), strings.NewReader(body), 20)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadLimitedBytes_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, err := textprocessor.ReadLimitedBytes(ctx, strings.NewReader("hello"), 10)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadLimitSubject_SandboxAndGenericFormats(t *testing.T) {
	t.Parallel()
	sandboxErr := fmt.Errorf("sandbox: stdout exceeds %d byte limit: %w", 4096, textprocessor.ErrReadLimitExceeded)
	require.Equal(t, "stdout", textprocessor.ReadLimitSubject(sandboxErr))

	genericErr := fmt.Errorf(
		"toolkit/web: response exceeds %d byte limit: %w",
		8192,
		textprocessor.ErrReadLimitExceeded,
	)
	require.Equal(t, "response", textprocessor.ReadLimitSubject(genericErr))
}

func TestReadLimitMaxBytes_SandboxAndGenericFormats(t *testing.T) {
	t.Parallel()
	sandboxErr := fmt.Errorf("sandbox: stdout exceeds %d byte limit: %w", 4096, textprocessor.ErrReadLimitExceeded)
	require.Equal(t, 4096, textprocessor.ReadLimitMaxBytes(sandboxErr))

	genericErr := fmt.Errorf("agents: stream exceeds 2048 byte limit: %w", textprocessor.ErrReadLimitExceeded)
	require.Equal(t, 2048, textprocessor.ReadLimitMaxBytes(genericErr))
}

func TestReadLimitedBytes_InfiniteReader(t *testing.T) {
	data, err := textprocessor.ReadLimitedBytes(context.Background(), infiniteReader{}, 1<<20)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadLimitedBytes_InfiniteReaderBoundedAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc smoke skipped in -short")
	}
	const limit = 1 << 20
	allocs := testing.AllocsPerRun(3, func() {
		data, err := textprocessor.ReadLimitedBytes(context.Background(), infiniteReader{}, limit)
		require.Error(t, err)
		require.Nil(t, data)
		require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	})
	require.Less(t, allocs, float64(64))
}

func TestReadLimitedBytes_InfiniteReaderFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped in -short")
	}
	start := time.Now()
	data, err := textprocessor.ReadLimitedBytes(context.Background(), infiniteReader{}, 1<<20)
	require.Less(t, time.Since(start), 200*time.Millisecond)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadLimitedBytes_DevZero(t *testing.T) {
	if testing.Short() {
		t.Skip("dev zero test skipped in -short")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/zero on Windows")
	}
	f, err := os.Open("/dev/zero")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	data, err := textprocessor.ReadLimitedBytes(context.Background(), f, 1<<20)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadLimitedBytes_NonPositiveLimit(t *testing.T) {
	data, err := textprocessor.ReadLimitedBytes(context.Background(), strings.NewReader("x"), 0)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadAndTruncate_TruncatesUTF8(t *testing.T) {
	body := strings.Repeat("x", 100)
	got, err := textprocessor.ReadAndTruncate(
		context.Background(),
		strings.NewReader(body),
		20,
		textprocessor.TruncationSuffix,
	)
	require.NoError(t, err)
	require.Contains(t, got, textprocessor.TruncationSuffix)
	require.LessOrEqual(t, len(got), 20+len(textprocessor.TruncationSuffix)+2)
}

func TestReadAndTruncate_MultibyteUTF8(t *testing.T) {
	body := "привет мир"
	got, err := textprocessor.ReadAndTruncate(context.Background(), strings.NewReader(body), 10, "...")
	require.NoError(t, err)
	require.True(t, utf8.ValidString(got))
}

func TestReadAndTruncate_ExceedDoesNotReturnSentinel(t *testing.T) {
	body := strings.Repeat("x", 100)
	got, err := textprocessor.ReadAndTruncate(
		context.Background(),
		strings.NewReader(body),
		20,
		textprocessor.TruncationSuffix,
	)
	require.NoError(t, err)
	require.Contains(t, got, textprocessor.TruncationSuffix)
	require.False(t, textprocessor.IsReadLimitExceeded(err))
}

func TestIsReadLimitExceeded(t *testing.T) {
	require.True(t, textprocessor.IsReadLimitExceeded(textprocessor.ErrReadLimitExceeded))
	require.True(t, textprocessor.IsReadLimitExceeded(
		fmt.Errorf("wrapped: %w", textprocessor.ErrReadLimitExceeded),
	))
	require.False(t, textprocessor.IsReadLimitExceeded(io.EOF))
	require.False(t, textprocessor.IsReadLimitExceeded(context.Canceled))
}

func TestReadLimitError_TypedParsing(t *testing.T) {
	t.Parallel()
	err := textprocessor.NewReadLimitError("stdout", 4096, textprocessor.ErrReadLimitExceeded)
	require.True(t, textprocessor.IsReadLimitExceeded(err))
	require.Equal(t, "stdout", textprocessor.ReadLimitSubject(err))
	require.Equal(t, 4096, textprocessor.ReadLimitMaxBytes(err))
}

func TestReadLimitedBytes_ReadError(t *testing.T) {
	data, err := textprocessor.ReadLimitedBytes(context.Background(), iotest.ErrReader(io.ErrUnexpectedEOF), 10)
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'z'
	}
	return len(p), nil
}
