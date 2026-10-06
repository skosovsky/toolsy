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
	const limit = 30
	got := truncatePayload(strings.Repeat("a", 100), limit)
	assert.Contains(t, got, payloadTruncatedSuffix)
	assert.True(t, utf8.ValidString(got))
	assert.Len(t, got, limit)
	assert.Equal(
		t,
		strings.Repeat("a", limit-len(payloadTruncatedSuffix)),
		strings.TrimSuffix(got, payloadTruncatedSuffix),
	)
}

func TestPayloadAccumulator_AppendsAndTruncates(t *testing.T) {
	t.Parallel()
	acc := newPayloadAccumulator(30)
	acc.append("12345")
	acc.append(strings.Repeat("x", 100))
	assert.Contains(t, acc.String(), payloadTruncatedSuffix)
	assert.Len(t, acc.String(), 30)
}

func TestPayloadLimits_ExactBytesWithMultibyteAndTinyBudgets(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, 2, 3, 7, 20, 30} {
		// Arrange.
		text := strings.Repeat("界", 100)
		acc := newPayloadAccumulator(limit)
		// Act.
		capped := truncatePayload(text, limit)
		acc.append(text[:9])
		acc.append(text[9:])
		// Assert.
		assert.LessOrEqual(t, len(capped), limit)
		assert.True(t, utf8.ValidString(capped))
		assert.LessOrEqual(t, len(acc.String()), limit)
		assert.True(t, utf8.ValidString(acc.String()))
	}
}
