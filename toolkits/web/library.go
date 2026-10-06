package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
)

// SearchStructured runs a search query and returns typed results (library mode).
func SearchStructured(
	ctx context.Context,
	provider SearchProvider,
	query string,
	opts ...Option,
) ([]SearchResult, error) {
	if provider == nil {
		return nil, toolsy.NewValidationError("search provider is required")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	return searchStructured(ctx, provider, query, &o)
}

func searchStructured(ctx context.Context, provider SearchProvider, query string, o *options) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, toolsy.NewValidationError("query is required")
	}
	results, err := provider.Search(ctx, query)
	if err != nil {
		return nil, toolsy.NewInternalError(fmt.Errorf("toolkit/web: search: %w", err))
	}
	if len(results) > o.maxSearchResults {
		return nil, toolsy.NewValidationError(
			"search result count exceeds limit; provider has no continuation contract",
		)
	}
	total := 2
	for i, result := range results {
		if len(result.Title) > o.maxSearchItemBytes || len(result.URL) > o.maxSearchItemBytes ||
			len(result.Snippet) > o.maxSearchItemBytes {
			return nil, toolsy.NewValidationError("search hit exceeds item byte limit")
		}
		raw, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return nil, toolsy.NewInternalError(marshalErr)
		}
		if len(raw) > o.maxSearchItemBytes {
			return nil, toolsy.NewValidationError("search hit exceeds item byte limit")
		}
		separator := 0
		if i > 0 {
			separator = 1
		}
		if len(raw)+separator > o.maxSearchSourceBytes-total {
			return nil, toolsy.NewValidationError("search results exceed source byte limit")
		}
		total += len(raw) + separator
	}
	return results, nil
}

// ScrapePage fetches a URL and returns main content as Markdown (library mode).
// HTML read and markdown conversion are fail-closed: pages or markdown output exceeding the derived
// byte cap return an error. Use WithMaxPageBytes to raise the budget (default 2MB wire budget).
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
	for _, r := range results {
		b.WriteString("- **")
		b.WriteString(escapeMarkdown(r.Title))
		b.WriteString("**: ")
		source := strings.TrimSpace(r.URL)
		if source == "" {
			source = "unavailable"
		}
		b.WriteString(source)
		if r.Snippet != "" {
			b.WriteString(" — ")
			b.WriteString(escapeMarkdown(r.Snippet))
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
