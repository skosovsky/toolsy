package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatContentBlocks_BinaryDataIsRetainedInProjection(t *testing.T) {
	// Arrange.
	blocks := []ContentBlock{
		{Type: "image", Data: "aW1hZ2U=", MIMEType: "image/png"},
		{Type: "audio", Data: "YXVkaW8=", MIMEType: "audio/wav"},
		{Type: "resource_link", URI: "file:///source.go", Name: "source.go"},
	}

	// Act.
	projection, err := FormatContentBlocks(blocks)

	// Assert.
	require.NoError(t, err)
	require.Contains(t, string(projection), "data:image/png;base64,aW1hZ2U=")
	require.Contains(t, string(projection), "data:audio/wav;base64,YXVkaW8=")
	require.Contains(t, string(projection), "[source.go](file:///source.go)")
}

func TestFormatContentBlocks_UnknownTypeFailsClosed(t *testing.T) {
	// Arrange.
	blocks := []ContentBlock{{Type: "future_type", Text: "must not be guessed"}}

	// Act.
	_, err := FormatContentBlocks(blocks)

	// Assert.
	require.Error(t, err)
}

func TestFormatResourceContents_RejectsAmbiguousUnion(t *testing.T) {
	// Arrange.
	text := "text"
	blob := "AA=="
	contents := []ResourceContents{{URI: "file:///x", Text: &text, Blob: &blob}}

	// Act.
	_, err := FormatResourceContents(contents)

	// Assert.
	require.Error(t, err)
}
