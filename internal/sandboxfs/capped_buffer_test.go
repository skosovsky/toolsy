package sandboxfs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestCappedBuffer_WriteExceedsLimit(t *testing.T) {
	t.Parallel()
	const maxBytes = 4
	buf := NewCappedBuffer("stdout", maxBytes)
	_, err := buf.Write([]byte("12345"))
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Equal(t, "stdout", ReadLimitSubject(err))
	require.Equal(t, maxBytes, ReadLimitMaxBytes(err))
	require.Contains(t, err.Error(), "exceeds 4 byte limit")
}
