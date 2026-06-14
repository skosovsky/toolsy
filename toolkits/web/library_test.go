package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

type stubProvider struct {
	results []SearchResult
	err     error
}

func (s *stubProvider) Search(_ context.Context, _ string) ([]SearchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func TestSearchStructured_ReturnsResults(t *testing.T) {
	provider := &stubProvider{results: []SearchResult{
		{Title: "A", URL: "https://a.example", Snippet: "snippet"},
	}}
	got, err := SearchStructured(context.Background(), provider, "query")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "A", got[0].Title)
}

func TestSearchStructured_EmptyQuery(t *testing.T) {
	_, err := SearchStructured(context.Background(), &stubProvider{}, "  ")
	require.Error(t, err)
}

func TestScrapePage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>Hello</p></body></html>"))
	}))
	defer srv.Close()

	md, err := ScrapePage(context.Background(), srv.URL, WithAllowPrivateIPs(true))
	require.NoError(t, err)
	require.Contains(t, md, "Hello")
}

func TestFormatSearchMarkdown(t *testing.T) {
	text := FormatSearchMarkdown([]SearchResult{
		{Title: "T|itle", URL: "https://x", Snippet: "s\nip"},
	})
	require.Contains(t, text, "T\\|itle")
	require.Contains(t, text, "https://x")
}

func TestFormatSearchMarkdown_TruncationSuffix(t *testing.T) {
	results := make([]SearchResult, 51)
	for i := range results {
		results[i] = SearchResult{Title: "T", URL: "https://x", Snippet: "s"}
	}
	text := FormatSearchMarkdown(results)
	require.Contains(t, text, strings.TrimSuffix(textprocessor.SearchResultsTruncationSuffix, "\n"))
}

func TestScrapePage_OversizedHTMLBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>" + strings.Repeat("x", 50000) + "</p></body></html>"))
	}))
	defer srv.Close()

	const maxPage = 4096
	md, err := ScrapePage(
		context.Background(),
		srv.URL,
		WithAllowPrivateIPs(true),
		WithMaxPageBytes(maxPage),
	)
	require.NoError(t, err)
	require.LessOrEqual(t, len(md), scrapeContentByteCap(maxPage))
	require.NotContains(t, md, "[Truncated]")
}

func TestScrapePage_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := ScrapePage(context.Background(), srv.URL, WithAllowPrivateIPs(true))
	require.Error(t, err)
}
