package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestRetrievalBoundsRejectProviderBeforeFormatter(t *testing.T) {
	for _, tc := range []struct {
		name string
		docs []Document
		opt  Option
	}{
		{"item escaping", []Document{{Content: strings.Repeat("<", 30)}}, WithMaxItemBytes(100)},
		{"item multibyte", []Document{{Content: strings.Repeat("界", 40)}}, WithMaxItemBytes(100)},
		{"metadata", []Document{{Metadata: map[string]string{"x": strings.Repeat("x", 200)}}}, WithMaxItemBytes(100)},
		{"source", []Document{{Content: "first"}, {Content: "second"}}, WithMaxSourceBytes(30)},
		{"count", make([]Document, 11), WithMaxResults(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			tool, err := AsSearchTool(
				&mockRetriever{docs: tc.docs},
				tc.opt,
				WithResultFormatter(func([]Document) (any, error) { called = true; return "ok", nil }),
			)
			require.NoError(t, err)
			err = tool.Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
				func(toolsy.Chunk) error { t.Fatal("must not yield"); return nil },
			)
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.False(t, called)
		})
	}
}

func TestDocumentIdentityAndProvenance(t *testing.T) {
	docs := []Document{
		{ID: "chunk-a", Content: "a", SourceURI: "doc://1"},
		{ID: "chunk-b", Content: "b", SourceURI: "doc://1"},
		{ID: "chunk-a", Content: "a", SourceURI: "doc://1"},
	}
	// Act: the thin renderer preserves host-selected units, including duplicates.
	markdown := FormatDocumentsMarkdown(docs)
	// Assert: routing/dedup policy belongs to the host, not the tool adapter.
	require.Equal(t, 3, strings.Count(markdown, "Source: doc://1"))
	require.Equal(t, 2, strings.Count(markdown, "ID: chunk-a"))
	require.Contains(t, markdown, "ID: chunk-b")
}

func TestDocumentWireLimitDoesNotDropProvenance(t *testing.T) {
	docs := []Document{
		{ID: "chunk-a", Content: strings.Repeat("<界", 20), SourceURI: "doc://1"},
		{ID: "chunk-b", Content: "b", SourceURI: "doc://1"},
	}
	tool, err := AsSearchTool(&mockRetriever{docs: docs}, WithResultShape(ShapeDocumentsJSON), WithMaxBytes(80))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { t.Fatal("must reject whole collection"); return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestRetrievalSourceBudgetExact(t *testing.T) {
	docs := []Document{{Content: "界"}, {Content: "<"}}
	raw, err := json.Marshal(docs)
	require.NoError(t, err)
	o := options{maxItemBytes: 100, maxSourceBytes: len(raw)}
	require.NoError(t, validateDocuments(docs, &o))
	o.maxSourceBytes--
	require.ErrorIs(t, validateDocuments(docs, &o), toolsy.ErrValidation)
}

func TestFormatterWireCapUsesActualHostDTO(t *testing.T) {
	tool, err := AsSearchTool(
		&mockRetriever{docs: []Document{{Content: strings.Repeat("x", 1000)}}},
		WithMaxBytes(100),
		WithResultFormatter(func(docs []Document) (any, error) { return map[string]int{"count": len(docs)}, nil }),
	)
	require.NoError(t, err)
	var raw []byte
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error { raw = c.Data; return nil },
	)
	require.NoError(t, err)
	require.JSONEq(t, `{"count":1}`, string(raw))
	require.LessOrEqual(t, len(raw), 100)
}

func TestEmptyUnitKeepsIdentifierAndHonestMissingSource(t *testing.T) {
	markdown := FormatDocumentsMarkdown([]Document{{ID: "empty-chunk"}})
	require.Contains(t, markdown, "ID: empty-chunk")
	require.Contains(t, markdown, "Source: unavailable")
	require.Contains(t, markdown, "empty content")
}

func TestHostDTOAdapterRetainsDeclaredPublicIdentifiers(t *testing.T) {
	// Arrange: the host selects the public identifiers; unrelated metadata is opaque.
	type hostHit struct {
		documentID string
		chunkID    string
		page       int
		content    string
		uri        string
	}
	hit := hostHit{
		documentID: "doc-7",
		chunkID:    "chunk-2",
		page:       4,
		content:    "answer",
		uri:        "https://example.com/manual",
	}
	unit := Document{
		ID:        fmt.Sprintf("document=%s; chunk=%s; page=%d", hit.documentID, hit.chunkID, hit.page),
		Content:   hit.content,
		SourceURI: hit.uri,
		Metadata:  map[string]string{"internal_note": "do not publish this in Markdown"},
	}
	// Act.
	markdown := FormatDocumentsMarkdown([]Document{unit})
	raw, err := json.Marshal(SearchDocumentsWire{Documents: []Document{unit}})
	// Assert: every public identifier survives, without automatic metadata exposure.
	require.NoError(t, err)
	for _, identifier := range []string{"document=doc-7", "chunk=chunk-2", "page=4", hit.uri} {
		require.Contains(t, markdown, identifier)
		require.Contains(t, string(raw), identifier)
	}
	require.NotContains(t, markdown, "internal_note")
	require.NotContains(t, markdown, "do not publish")
	require.Contains(t, string(raw), "internal_note")
}
