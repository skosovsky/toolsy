package document

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestWordMLInsertedWhitespaceInclusiveBudget(t *testing.T) {
	for _, node := range []string{"tab", "br", "cr"} {
		t.Run(node, func(t *testing.T) {
			// Arrange: decoded UTF-8 text consumes two bytes and separator consumes one.
			raw := []byte(`<document><p><r><t>é</t><` + node + `/></r></p></document>`)
			separator := "\n"
			if node == "tab" {
				separator = "\t"
			}
			// Act.
			text, err := extractTextFromWordXMLWithLimits(t.Context(), raw, Limits{ParsedBytes: 3, ItemBytes: 2})
			// Assert.
			require.NoError(t, err)
			require.Equal(t, "é"+separator, text)
			text, err = extractTextFromWordXMLWithLimits(t.Context(), raw, Limits{ParsedBytes: 2, ItemBytes: 2})
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Empty(t, text)
		})
	}
}
func TestWordMLParagraphAndLeadingWhitespaceBudget(t *testing.T) {
	for _, raw := range []string{`<document><p><t>a</t></p><p><t>b</t></p></document>`, `<document><p><tab/><br/><cr/></p></document>`} {
		// Arrange/Act: separators count even when no text precedes them.
		text, err := extractTextFromWordXMLWithLimits(t.Context(), []byte(raw), Limits{ParsedBytes: 3, ItemBytes: 1})
		// Assert.
		require.NoError(t, err)
		require.Len(t, text, 3)
		text, err = extractTextFromWordXMLWithLimits(t.Context(), []byte(raw), Limits{ParsedBytes: 2, ItemBytes: 1})
		require.ErrorIs(t, err, toolsy.ErrValidation)
		require.Empty(t, text)
	}
}
func TestWordMLSeparatorCancellationPrecedesOverflow(t *testing.T) {
	// Arrange.
	var builder strings.Builder
	builder.WriteString("a")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Act.
	err := appendWordMLSeparator(ctx, &builder, 1, '\t')
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "a", builder.String())
}
