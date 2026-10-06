package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Aggregate runs retrievers in order and concatenates results.
func Aggregate(retrievers ...DocumentRetriever) DocumentRetriever {
	return documentRetrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
		var out []Document
		for _, r := range retrievers {
			if r == nil {
				continue
			}
			docs, err := r.Retrieve(ctx, query)
			if err != nil {
				return nil, err
			}
			out = append(out, docs...)
		}
		return out, nil
	})
}

// Dedup removes identical retrieval units, preserving distinct chunks of one source.
func Dedup(retriever DocumentRetriever) DocumentRetriever {
	return DedupBy(retriever, documentDedupKey)
}

// DedupBy uses a host identity function. Empty keys are retained without deduplication.
func DedupBy(retriever DocumentRetriever, identity func(Document) string) DocumentRetriever {
	if retriever == nil || identity == nil {
		return nil
	}
	return documentRetrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
		docs, err := retriever.Retrieve(ctx, query)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]struct{}, len(docs))
		out := make([]Document, 0, len(docs))
		for _, doc := range docs {
			key := identity(doc)
			if key == "" {
				out = append(out, doc)
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, doc)
		}
		return out, nil
	})
}

// Fallback tries primary; on error or empty result, uses secondary.
func Fallback(primary, secondary DocumentRetriever) DocumentRetriever {
	return documentRetrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
		if primary != nil {
			docs, err := primary.Retrieve(ctx, query)
			if err == nil && len(docs) > 0 {
				return docs, nil
			}
		}
		if secondary == nil {
			return nil, nil
		}
		return secondary.Retrieve(ctx, query)
	})
}

type documentRetrieverFunc func(ctx context.Context, query string) ([]Document, error)

func (f documentRetrieverFunc) Retrieve(ctx context.Context, query string) ([]Document, error) {
	return f(ctx, query)
}

func documentDedupKey(doc Document) string {
	raw, _ := json.Marshal(doc)
	return string(raw)
}

// FormatDocumentsMarkdown renders documents as numbered Markdown for LLM consumption.
func FormatDocumentsMarkdown(docs []Document) string {
	if len(docs) == 0 {
		return "No results found."
	}
	var b strings.Builder
	n := 1
	for _, doc := range docs {
		content := strings.TrimSpace(doc.Content)
		if content == "" {
			content = "(empty content)"
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		source := strings.TrimSpace(doc.SourceURI)
		if source == "" {
			source = "unavailable"
		}
		_, _ = fmt.Fprintf(&b, "%d. %s\n   Source: %s", n, content, source)
		if doc.ID != "" {
			_, _ = fmt.Fprintf(&b, "\n   ID: %s", doc.ID)
		}
		n++
	}
	if b.Len() == 0 {
		return "No results found."
	}
	return b.String()
}
