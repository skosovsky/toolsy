package rag

import "context"

// Document is the structured retrieval unit exchanged between retrievers and the search tool.
type Document struct {
	// ID is the host-selected public unit identifier, independent of source location.
	// Map any identifiers intended for citation here; no metadata keys are reserved.
	ID        string `json:"id,omitempty"`
	Content   string `json:"content"`
	SourceURI string `json:"source_uri,omitempty"`
	Category  string `json:"category,omitempty"`
	// Metadata is opaque host data, omitted from default Markdown but included in JSON.
	// Hosts must omit sensitive metadata or map to a public DTO before JSON publication.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DocumentRetriever returns structured documents for a query.
type DocumentRetriever interface {
	Retrieve(ctx context.Context, query string) ([]Document, error)
}
