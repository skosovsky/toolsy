package document

import (
	"net/http"
)

// HTTPClient is the minimal HTTP surface used for remote document fetch. Pass [*http.Client] with Timeout only;
// Transport is always merged from the default SSRF-safe client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Option configures AsTool (limits, remote fetch, tool name).
type Option func(*options)

type options struct {
	maxBytes            int
	allowRemote         bool
	allowPrivateIPs     bool
	httpClient          HTTPClient
	toolName            string
	toolDesc            string
	resultFormatter     func(ExtractWireResult) (any, error)
	hostResultValidator func(any) error
}

const (
	defaultMaxBytes = 2 * 1024 * 1024 // 2 MB
	defaultToolName = "document_extract_text"
	defaultToolDesc = "Extract text from a document (PDF, CSV, DOCX) by file path or URL"
)

func applyDefaults(o *options) {
	if o.maxBytes <= 0 {
		o.maxBytes = defaultMaxBytes
	}
	if o.toolName == "" {
		o.toolName = defaultToolName
	}
	if o.toolDesc == "" {
		o.toolDesc = defaultToolDesc
	}
}

// WithMaxBytes sets the wire JSON byte budget (default 2 MB). Local stat, remote download, and parsers
// use contentByteCap(maxBytes) for fail-closed reads; wire suffix applies separately via format.CapWireJSON.
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

// WithHTTPClient sets the HTTP client for URL downloads. Only Timeout is merged onto the default SSRF-safe client.
func WithHTTPClient(c HTTPClient) Option {
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

// WithResultFormatter overrides JSON output for document extraction.
func WithResultFormatter(f func(ExtractWireResult) (any, error)) Option {
	return func(o *options) {
		o.resultFormatter = f
	}
}

// WithHostResultValidator validates formatted tool output before JSON marshal.
func WithHostResultValidator(v func(any) error) Option {
	return func(o *options) {
		o.hostResultValidator = v
	}
}
