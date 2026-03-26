package rag

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type mockRetriever struct {
	results []string
	err     error
}

func (m *mockRetriever) Retrieve(ctx context.Context, query string) ([]string, error) {
	_ = ctx
	_ = query
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func decodeSearchResult(t *testing.T, c toolsy.Chunk) searchResult {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out searchResult
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestAsSearchTool_FormatsMarkdown(t *testing.T) {
	r := &mockRetriever{results: []string{"a", "b", "c"}}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	var result string
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.RunContext{},
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
			func(c toolsy.Chunk) error {
				result = decodeSearchResult(t, c).Results
				return nil
			},
		),
	)
	require.Equal(t, "1. a\n2. b\n3. c", result)
}

func TestAsSearchTool_MaxResults(t *testing.T) {
	r := &mockRetriever{results: []string{"a", "b", "c"}}
	tool, err := AsSearchTool(r, WithMaxResults(2))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchResult(t, c).Results
			return nil
		},
	)
	require.Equal(t, "1. a\n2. b", result)
}

func TestAsSearchTool_MaxBytesTruncate(t *testing.T) {
	r := &mockRetriever{results: []string{strings.Repeat("x", 100)}}
	tool, err := AsSearchTool(r, WithMaxBytes(20))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchResult(t, c).Results
			return nil
		},
	)
	require.True(t, strings.HasSuffix(result, "[Truncated]"), "expected [Truncated] suffix, got %q", result)
}

func TestAsSearchTool_MaxBytesUTF8Safe(t *testing.T) {
	// Cyrillic is multi-byte; truncating in the middle must not break UTF-8
	r := &mockRetriever{results: []string{"привет мир"}}
	tool, err := AsSearchTool(r, WithMaxBytes(17))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchResult(t, c).Results
			return nil
		},
	)
	require.True(t, strings.HasSuffix(result, "[Truncated]"), "expected [Truncated] suffix, got %q", result)
	require.True(t, utf8.ValidString(result), "expected valid UTF-8, got %q", result)
	require.Contains(t, result, "1. п")
}

func TestAsSearchTool_RetrieverError(t *testing.T) {
	r := &mockRetriever{err: errors.New("backend down")}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	// Core wraps handler errors in SystemError; the underlying cause should contain our prefix
	cause := err
	for cause != nil {
		if strings.Contains(cause.Error(), "toolkit/rag:") {
			return
		}
		cause = errors.Unwrap(cause)
	}
	t.Errorf("expected toolkit/rag in error chain, got %q", err.Error())
}

func TestAsSearchTool_EmptyResult(t *testing.T) {
	r := &mockRetriever{results: nil}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	var result string
	_ = tool.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"x"}`)},
		func(c toolsy.Chunk) error {
			result = decodeSearchResult(t, c).Results
			return nil
		},
	)
	require.Equal(t, "No results found.", result)
}
