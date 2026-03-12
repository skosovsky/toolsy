package document

import (
	"net/http"
	"time"
)

// Option configures AsTool (limits, remote fetch, tool name).
type Option func(*options)

type options struct {
	maxBytes        int
	allowRemote     bool
	allowPrivateIPs bool // for tests (e.g. httptest on 127.0.0.1); default false for SSRF safety
	httpClient      *http.Client
	toolName        string
	toolDesc        string
}

const (
	defaultMaxBytes = 2 * 1024 * 1024 // 2 MB
	defaultToolName = "document_extract_text"
	defaultToolDesc = "Extract text from a document (PDF, CSV, DOCX) by file path or URL"
)

const defaultHTTPTimeout = 30 * time.Second

func applyDefaults(o *options) {
	if o.maxBytes <= 0 {
		o.maxBytes = defaultMaxBytes
	}
	if o.httpClient == nil {
		o.httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if o.toolName == "" {
		o.toolName = defaultToolName
	}
	if o.toolDesc == "" {
		o.toolDesc = defaultToolDesc
	}
}

// WithMaxBytes sets the maximum file size to process and output truncation limit (default 2 MB).
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

// WithAllowRemote enables fetching documents by URL (default false).
func WithAllowRemote(allow bool) Option {
	return func(o *options) {
		o.allowRemote = allow
	}
}

// WithAllowPrivateIPs allows fetching from loopback/link-local/private IPs (default false).
// Use only for tests (e.g. httptest); production should keep false for SSRF safety.
func WithAllowPrivateIPs(allow bool) Option {
	return func(o *options) {
		o.allowPrivateIPs = allow
	}
}

// WithHTTPClient sets the HTTP client for URL downloads (e.g. for SSRF-safe transport).
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) {
		o.httpClient = c
	}
}

// WithToolName sets the name of the extract tool.
func WithToolName(name string) Option {
	return func(o *options) {
		o.toolName = name
	}
}

// WithToolDescription sets the description of the extract tool.
func WithToolDescription(desc string) Option {
	return func(o *options) {
		o.toolDesc = desc
	}
}
