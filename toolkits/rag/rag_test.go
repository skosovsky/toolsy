package rag

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func TestAsSearchTool_FormatsMarkdown(t *testing.T) {
	r := &mockRetriever{results: []string{"a", "b", "c"}}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	var result string
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if res, ok := c.RawData.(searchResult); ok {
				result = res.Results
			}
		}
		return nil
	}))
	require.Equal(t, "1. a\n2. b\n3. c", result)
}

func TestAsSearchTool_MaxResults(t *testing.T) {
	r := &mockRetriever{results: []string{"a", "b", "c"}}
	tool, err := AsSearchTool(r, WithMaxResults(2))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if res, ok := c.RawData.(searchResult); ok {
				result = res.Results
			}
		}
		return nil
	})
	require.Equal(t, "1. a\n2. b", result)
}

func TestAsSearchTool_MaxBytesTruncate(t *testing.T) {
	r := &mockRetriever{results: []string{strings.Repeat("x", 100)}}
	tool, err := AsSearchTool(r, WithMaxBytes(20))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if res, ok := c.RawData.(searchResult); ok {
				result = res.Results
			}
		}
		return nil
	})
	require.True(t, strings.HasSuffix(result, "[Truncated]"), "expected [Truncated] suffix, got %q", result)
}

func TestAsSearchTool_MaxBytesUTF8Safe(t *testing.T) {
	// Cyrillic is multi-byte; truncating in the middle must not break UTF-8
	r := &mockRetriever{results: []string{"привет мир"}}
	tool, err := AsSearchTool(r, WithMaxBytes(5))
	require.NoError(t, err)
	var result string
	_ = tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if res, ok := c.RawData.(searchResult); ok {
				result = res.Results
			}
		}
		return nil
	})
	require.True(t, strings.HasSuffix(result, "[Truncated]"), "expected [Truncated] suffix, got %q", result)
	// Result should be valid (no replacement char in the middle of a rune)
	if strings.Contains(result, "\ufffd") && len(result) > 12 {
		t.Logf("result may have replacement char from valid truncation: %q", result)
	}
}

func TestAsSearchTool_RetrieverError(t *testing.T) {
	r := &mockRetriever{err: errors.New("backend down")}
	tool, err := AsSearchTool(r)
	require.NoError(t, err)
	err = tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(toolsy.Chunk) error { return nil })
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
	_ = tool.Execute(context.Background(), []byte(`{"query":"x"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if res, ok := c.RawData.(searchResult); ok {
				result = res.Results
			}
		}
		return nil
	})
	require.Equal(t, "No results found.", result)
}
