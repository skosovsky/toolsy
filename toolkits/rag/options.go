package rag

import (
	"context"
)

const defaultMaxBytes = 512 * 1024
const defaultMaxResults = 10
const defaultMaxItemBytes = 64 * 1024

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

// Option rejects nil at construction. Negative limits reject; zero selects finite defaults.
// Host ports and callbacks are borrowed; the host owns their lifetime and synchronization.
// Option configures the search tool.
type Option func(*options)

type options struct {
	name                string
	description         string
	maxBytes            int
	maxResults          int
	maxItemBytes        int
	maxSourceBytes      int
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

// WithMaxBytes limits final encoded wire JSON, including its envelope (default 512 KiB).
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

// WithMaxResults sets the maximum number of results to include (zero = default 10; negative rejects construction).
func WithMaxResults(n int) Option {
	return func(o *options) {
		o.maxResults = n
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
	if o.maxItemBytes == 0 {
		o.maxItemBytes = defaultMaxItemBytes
	}
	if o.maxSourceBytes == 0 {
		o.maxSourceBytes = defaultMaxBytes
	}
	if o.name == "" {
		o.name = "search_knowledge_base"
	}
	if o.description == "" {
		o.description = "Search the knowledge base for relevant information"
	}
	if o.maxBytes == 0 {
		o.maxBytes = defaultMaxBytes
	}
	if o.maxResults == 0 {
		o.maxResults = defaultMaxResults
	}
}

// WithMaxItemBytes limits each encoded provider unit (default 64 KiB).
func WithMaxItemBytes(n int) Option { return func(o *options) { o.maxItemBytes = n } }

// WithMaxSourceBytes limits the total encoded provider collection (default 512 KiB).
func WithMaxSourceBytes(n int) Option { return func(o *options) { o.maxSourceBytes = n } }
