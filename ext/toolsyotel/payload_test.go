package toolsyotel

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncatePayload_NoTruncation(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", truncatePayload("hello", 10))
}

func TestTruncatePayload_TruncatesWithSuffix(t *testing.T) {
	t.Parallel()
	const limit = 10
	got := truncatePayload(strings.Repeat("a", 100), limit)
	assert.Contains(t, got, payloadTruncatedSuffix)
	assert.True(t, utf8.ValidString(got))
	assert.Len(t, got, limit+len(payloadTruncatedSuffix))
	assert.Equal(t, strings.Repeat("a", limit), got[:limit])
}

func TestPayloadAccumulator_AppendsAndTruncates(t *testing.T) {
	t.Parallel()
	acc := newPayloadAccumulator(10)
	acc.append("12345")
	acc.append("67890EXTRA")
	assert.Contains(t, acc.String(), payloadTruncatedSuffix)
	assert.Len(t, acc.String(), 10+len(payloadTruncatedSuffix))
}
