package rag

const defaultMaxBytes = 512 * 1024
const defaultMaxResults = 10

// Option configures the search tool.
type Option func(*options)

type options struct {
	name        string
	description string
	maxBytes    int
	maxResults  int
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
	if o.maxResults <= 0 {
		o.maxResults = defaultMaxResults
	}
}
