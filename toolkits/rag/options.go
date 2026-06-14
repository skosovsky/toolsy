package rag

import (
	"context"
)

const defaultMaxBytes = 512 * 1024
const defaultMaxResults = 10

// ResultShape controls default tool output encoding.
type ResultShape int

const (
	// ShapeMarkdown returns numbered Markdown in {"results": "..."}.
	ShapeMarkdown ResultShape = iota
	// ShapeDocumentsJSON returns {"documents": [...]}.
	ShapeDocumentsJSON
)

// ScopeFilter removes documents the current user may not access (RBAC hook).
type ScopeFilter func(ctx context.Context, docs []Document) []Document

// Option configures the search tool.
type Option func(*options)

type options struct {
	name                string
	description         string
	maxBytes            int
	maxResults          int
	maxResultsSet       bool
	resultShape         ResultShape
	scopeFilter         ScopeFilter
	resultFormatter     func([]Document) (any, error)
	hostResultValidator func(any) error
}

// WithName sets the tool name (default: "search_knowledge_base").
func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

// WithDescription sets the tool description.
func WithDescription(description string) Option {
	return func(o *options) {
		o.description = description
	}
}

// WithMaxBytes sets the maximum length of the combined result text in bytes (default: 512KB).
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

// WithMaxResults sets the maximum number of results to include (0 = no limit).
func WithMaxResults(n int) Option {
	return func(o *options) {
		o.maxResults = n
		o.maxResultsSet = true
	}
}

// WithResultShape sets default output shape (Markdown or Documents JSON).
func WithResultShape(shape ResultShape) Option {
	return func(o *options) {
		o.resultShape = shape
	}
}

// WithScopeFilter filters retrieved documents before formatting (RBAC / tenancy).
func WithScopeFilter(f ScopeFilter) Option {
	return func(o *options) {
		o.scopeFilter = f
	}
}

// WithResultFormatter overrides the JSON returned to the host/LLM.
func WithResultFormatter(f func([]Document) (any, error)) Option {
	return func(o *options) {
		o.resultFormatter = f
	}
}

// WithHostResultValidator validates formatted output before JSON marshal.
func WithHostResultValidator(v func(any) error) Option {
	return func(o *options) {
		o.hostResultValidator = v
	}
}

func (o *options) applyDefaults() {
	if o.name == "" {
		o.name = "search_knowledge_base"
	}
	if o.description == "" {
		o.description = "Search the knowledge base for relevant information"
	}
	if o.maxBytes <= 0 {
		o.maxBytes = defaultMaxBytes
	}
	if !o.maxResultsSet {
		o.maxResults = defaultMaxResults
	}
}
