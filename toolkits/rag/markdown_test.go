package rag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Retained renderer regression from the removed router test file.
func TestFormatDocumentsMarkdown_Empty(t *testing.T) {
	// Arrange.
	var docs []Document
	// Act.
	got := FormatDocumentsMarkdown(docs)
	// Assert.
	require.Equal(t, "No results found.", got)
}
