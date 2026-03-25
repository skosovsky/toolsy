package textutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateStringUTF8(t *testing.T) {
	got := TruncateStringUTF8("привет мир", 8, "[x]")
	require.Equal(t, "пр[x]", got)
}

func TestTruncateStringUTF8_TinyLimitTruncatesSuffix(t *testing.T) {
	got := TruncateStringUTF8("abcdef", 2, "[x]")
	require.Equal(t, "[x"[:2], got)
	require.LessOrEqual(t, len(got), 2)
}

func TestTruncateStringUTF8_ZeroOrNegativeLimit(t *testing.T) {
	require.Empty(t, TruncateStringUTF8("abcdef", 0, "[x]"))
	require.Empty(t, TruncateStringUTF8("abcdef", -1, "[x]"))
}

func TestTruncateStringUTF8_MultibyteSuffix(t *testing.T) {
	got := TruncateStringUTF8("abcdef", 5, "✓✓")
	require.Equal(t, "ab✓", got)
	require.LessOrEqual(t, len(got), 5)
}

func TestTruncateBytesToValidUTF8String(t *testing.T) {
	data := []byte("hello\xffworld")
	got := TruncateBytesToValidUTF8String(data, 7, "[x]")
	require.Equal(t, "hellow[x]", got)
}

func TestTruncateBytesToValidUTF8String_ZeroOrNegativeLimit(t *testing.T) {
	require.Empty(t, TruncateBytesToValidUTF8String([]byte("abcdef"), 0, "[x]"))
	require.Empty(t, TruncateBytesToValidUTF8String([]byte("abcdef"), -1, "[x]"))
}

func TestReadAndTruncateValidUTF8(t *testing.T) {
	got, err := ReadAndTruncateValidUTF8(bytes.NewBufferString("abcdef"), 4, "[x]")
	require.NoError(t, err)
	require.Equal(t, "abcd[x]", got)
}

func TestReadAndTruncateValidUTF8_ZeroOrNegativeLimit(t *testing.T) {
	got, err := ReadAndTruncateValidUTF8(bytes.NewBufferString("abcdef"), 0, "[x]")
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = ReadAndTruncateValidUTF8(bytes.NewBufferString("abcdef"), -1, "[x]")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestTruncateBytes(t *testing.T) {
	got := TruncateBytes([]byte("abcdef"), 4, "[x]")
	require.Equal(t, []byte("abcd[x]"), got)
}

func TestTruncateBytes_ZeroOrNegativeLimit(t *testing.T) {
	require.Empty(t, TruncateBytes([]byte("abcdef"), 0, "[x]"))
	require.Empty(t, TruncateBytes([]byte("abcdef"), -1, "[x]"))
}
