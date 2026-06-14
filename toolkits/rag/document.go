package rag

import "context"

// Document is the structured retrieval unit exchanged between retrievers and the search tool.
type Document struct {
	Content   string            `json:"content"`
	SourceURI string            `json:"source_uri,omitempty"`
	Category  string            `json:"category,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// DocumentRetriever returns structured documents for a query.
type DocumentRetriever interface {
	Retrieve(ctx context.Context, query string) ([]Document, error)
}
