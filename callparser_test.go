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
	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", ArgsChunk: `"go"}`},
	}

	// Act.
	raw, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{"q":"go"}`, string(raw))
}

func TestStandardCallParser_NormalizesContinuationMissingToolName(t *testing.T) {
	t.Parallel()
	type args struct {
		Q string `json:"q"`
	}

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
		{Type: ContentTypeToolCall, ToolCallID: "call_1", ArgsChunk: `"go"}`},
	}

	// Act.
	raw, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")
	decoded, decodeErr := ParseExactlyOne[args](parts, "search")

	// Assert.
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	require.JSONEq(t, `{"q":"go"}`, string(raw))
	require.Equal(t, "go", decoded.Q)
	require.Empty(t, parts[1].ToolName, "normalization must not mutate caller-owned parts")
}

func TestStandardCallParser_Multiple(t *testing.T) {
	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", Args: `{}`},
		{Type: ContentTypeToolCall, ToolName: "search", Args: `{}`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
}

func TestStandardCallParser_RejectsDuplicateCompleteCallWithSameID(t *testing.T) {
	t.Parallel()

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":"go"}`},
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":"go"}`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestStandardCallParser_RejectsSplitCompleteThenDuplicateSameID(t *testing.T) {
	t.Parallel()

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", ArgsChunk: `"go"}`},
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":"again"}`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestStandardCallParser_RejectsInvalidMergedJSON(t *testing.T) {
	t.Parallel()

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestStandardCallParser_RejectsOtherToolCallWhenExpectingExactlyOne(t *testing.T) {
	t.Parallel()

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{}`},
		{Type: ContentTypeToolCall, ToolName: "other", ToolCallID: "call_2", Args: `{}`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
	requireClientCorrectable(t, err)
}

func TestStandardCallParser_RejectsConflictingNameForSameCallID(t *testing.T) {
	t.Parallel()

	// Arrange.
	parts := []ContentPart{
		{Type: ContentTypeToolCall, ToolName: "search", ToolCallID: "call_1", Args: `{"q":`},
		{Type: ContentTypeToolCall, ToolName: "other", ToolCallID: "call_1", ArgsChunk: `"go"}`},
	}

	// Act.
	_, err := StandardCallParser{}.ExtractExactlyOne(parts, "search")

	// Assert.
	require.Error(t, err)
	requireClientCorrectable(t, err)
}
