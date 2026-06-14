package httptool

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestDrainResponseBody_CapsTail(t *testing.T) {
	tail := strings.Repeat("x", DefaultMaxDrainBytes+1024)
	r := strings.NewReader(tail)
	err := DrainResponseBody(context.Background(), r, DefaultMaxDrainBytes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "drain exceeds")
}

func TestDrainResponseBody_ShortBody(t *testing.T) {
	r := strings.NewReader("hello")
	require.NoError(t, DrainResponseBody(context.Background(), r, DefaultMaxDrainBytes))
}

func TestLimitStreamReader_CapsTotal(t *testing.T) {
	r := strings.NewReader(strings.Repeat("a", 100))
	limited := LimitStreamReader(r, 50)
	buf := make([]byte, 100)
	n, err := limited.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 50, n)
	_, err = limited.Read(buf)
	require.Error(t, err)
}

func TestReadBodyLimited_TruncatesUTF8(t *testing.T) {
	body := strings.Repeat("x", 100)
	got, err := ReadBodyLimited(context.Background(), strings.NewReader(body), 20)
	require.NoError(t, err)
	require.Contains(t, got, truncationSuffix)
	require.LessOrEqual(t, len(got), 20+len(truncationSuffix)+2)
}

func TestReadBodyLimited_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadBodyLimited(ctx, strings.NewReader("hello"), 10)
	require.Error(t, err)
}

func TestReadBodyLimited_ShortBody(t *testing.T) {
	got, err := ReadBodyLimited(context.Background(), strings.NewReader("hello"), 100)
	require.NoError(t, err)
	require.Equal(t, "hello", got)
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

func TestReadBodyLimited_MultibyteUTF8(t *testing.T) {
	body := strings.Repeat("ж", 50)
	got, err := ReadBodyLimited(context.Background(), strings.NewReader(body), 10)
	require.NoError(t, err)
	require.Contains(t, got, truncationSuffix)
	require.True(t, utf8.ValidString(got))
}
