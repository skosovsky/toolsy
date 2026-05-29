package textprocessor_test

import (
	"testing"
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
