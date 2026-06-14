package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

// SearchStructured runs a search query and returns typed results (library mode).
func SearchStructured(ctx context.Context, provider SearchProvider, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, toolsy.NewValidationError("query is required")
	}
	results, err := provider.Search(ctx, query)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/web: search: %w", err))
	}
	if len(results) > maxSearchResultsDisplayed {
		results = results[:maxSearchResultsDisplayed]
	}
	return results, nil
}

// ScrapePage fetches a URL and returns main content as Markdown (library mode).
// HTML is pre-capped without a truncation suffix; there is no JSON wire envelope.
// WithMaxPageBytes sets the wire budget used to derive the HTML read limit (see scrapeContentByteCap).
func ScrapePage(ctx context.Context, rawURL string, opts ...Option) (string, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	result, err := doScrape(ctx, &o, rawURL)
	if err != nil {
		return "", err
	}
	return result.Markdown, nil
}

// FormatSearchMarkdown formats search hits as Markdown for LLM consumption.
func FormatSearchMarkdown(results []SearchResult) string {
	var b strings.Builder
	for i, r := range results {
		b.WriteString("- **")
		b.WriteString(escapeMarkdown(r.Title))
		b.WriteString("**: ")
		b.WriteString(r.URL)
		if r.Snippet != "" {
			b.WriteString(" — ")
			b.WriteString(escapeMarkdown(r.Snippet))
		}
		b.WriteString("\n")
		if i+1 == maxSearchResultsDisplayed {
			b.WriteString(textprocessor.SearchResultsTruncationSuffix)
			break
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
