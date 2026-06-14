package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDedup_BySourceURI(t *testing.T) {
	base := &mockRetriever{docs: []Document{
		{Content: "a", SourceURI: "doc://1"},
		{Content: "b", SourceURI: "doc://1"},
		{Content: "c", SourceURI: "doc://2"},
	}}
	got, err := Dedup(base).Retrieve(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestDedup_ByContentFNV(t *testing.T) {
	base := &mockRetriever{docs: []Document{
		{Content: "same"},
		{Content: "same"},
		{Content: "other"},
	}}
	got, err := Dedup(base).Retrieve(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestFallback_UsesSecondaryWhenPrimaryEmpty(t *testing.T) {
	primary := &mockRetriever{docs: nil}
	secondary := &mockRetriever{docs: docsFromStrings("fallback")}
	got, err := Fallback(primary, secondary).Retrieve(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "fallback", got[0].Content)
}

func TestFallback_PrimaryError_UsesSecondary(t *testing.T) {
	primary := &mockRetriever{err: errors.New("primary down")}
	secondary := &mockRetriever{docs: docsFromStrings("fallback")}
	got, err := Fallback(primary, secondary).Retrieve(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "fallback", got[0].Content)
}

func TestDedup_NilRetriever(t *testing.T) {
	require.Nil(t, Dedup(nil))
}

func TestAggregate_Concatenates(t *testing.T) {
	a := &mockRetriever{docs: docsFromStrings("a")}
	b := &mockRetriever{docs: docsFromStrings("b")}
	got, err := Aggregate(a, b).Retrieve(context.Background(), "q")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestFormatDocumentsMarkdown_Empty(t *testing.T) {
	require.Equal(t, "No results found.", FormatDocumentsMarkdown(nil))
}

func TestDocumentDedupKey_URI(t *testing.T) {
	require.Equal(t, "uri:doc://x", documentDedupKey(Document{Content: "a", SourceURI: "doc://x"}))
}

func TestDocumentDedupKey_FNV(t *testing.T) {
	k1 := documentDedupKey(Document{Content: "hello"})
	k2 := documentDedupKey(Document{Content: "hello"})
	k3 := documentDedupKey(Document{Content: "other"})
	require.Equal(t, k1, k2)
	require.NotEqual(t, k1, k3)
	require.Contains(t, k1, "fnv:")
}
