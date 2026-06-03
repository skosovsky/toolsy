package toolsy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardCallParser_ExtractExactlyOne(t *testing.T) {
	parser := StandardCallParser{}
	parts := []ContentPart{
		{Type: ContentTypeText, Text: "thinking"},
		{Type: ContentTypeToolCall, ToolName: "search", Args: `{"q":`, ArgsChunk: `"go"}`},
	}
	raw, err := parser.ExtractExactlyOne(parts, "search")
	require.NoError(t, err)
	require.JSONEq(t, `{"q":"go"}`, string(raw))
}

func TestStandardCallParser_NoMatch(t *testing.T) {
	_, err := StandardCallParser{}.ExtractExactlyOne([]ContentPart{
		{Type: ContentTypeToolCall, ToolName: "other", Args: `{}`},
	}, "search")
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestStandardCallParser_SameToolCallID_Merged(t *testing.T) {
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", ArgsChunk: `"go"}`},
	}
	raw, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")
	require.NoError(t, err)
	require.JSONEq(t, `{"q":"go"}`, string(raw))
}

func TestStandardCallParser_Multiple(t *testing.T) {
	_, err := StandardCallParser{}.ExtractExactlyOne([]ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", Args: `{}`},
		{Type: ContentTypeToolCall, ToolName: "search", Args: `{}`},
	}, "search")
	require.Error(t, err)
}
