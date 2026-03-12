package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
)

// Retriever is the interface the toolkit expects. Implement it with any backend
// (vector DB, ragy, etc.); the toolkit only needs query -> []string.
type Retriever interface {
	Retrieve(ctx context.Context, query string) ([]string, error)
}

type searchArgs struct {
	Query string `json:"query"`
}

type searchResult struct {
	Results string `json:"results"`
}

// AsSearchTool builds a single toolsy.Tool that calls r.Retrieve, formats results
// as numbered Markdown, and optionally truncates by maxBytes (UTF-8 safe).
func AsSearchTool(r Retriever, opts ...Option) (toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	o.applyDefaults()

	handler := func(ctx context.Context, args searchArgs) (searchResult, error) {
		results, err := r.Retrieve(ctx, args.Query)
		if err != nil {
			return searchResult{}, fmt.Errorf("toolkit/rag: retrieve failed: %w", err)
		}
		if len(results) == 0 {
			return searchResult{Results: "No results found."}, nil
		}
		if o.maxResults > 0 && len(results) > o.maxResults {
			results = results[:o.maxResults]
		}
		var b strings.Builder
		for i, s := range results {
			_, _ = fmt.Fprintf(&b, "%d. %s\n", i+1, s)
		}
		text := strings.TrimSuffix(b.String(), "\n")
		if o.maxBytes > 0 && len(text) > o.maxBytes {
			trunc := text[:o.maxBytes]
			trunc = strings.ToValidUTF8(trunc, "")
			text = trunc + "\n[Truncated]"
		}
		return searchResult{Results: text}, nil
	}

	return toolsy.NewTool(o.name, o.description, handler)
}
