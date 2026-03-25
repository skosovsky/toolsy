package httptool

import (
	"net/http"
	"strings"
)

// Option configures AsTools (client, allowed domains, headers, limits, names).
type Option func(*options)

type options struct {
	httpClient      *http.Client
	allowedDomains  []string
	headers         map[string]string
	maxResponseBody int
	getName         string
	getDesc         string
	postName        string
	postDesc        string
	allowPrivateIPs bool // internal use for tests only
}

const defaultMaxResponseBody = 512 * 1024

func applyDefaults(o *options) {
	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
	}
	if o.maxResponseBody <= 0 {
		o.maxResponseBody = defaultMaxResponseBody
	}
	if o.getName == "" {
		o.getName = "http_get"
	}
	if o.getDesc == "" {
		o.getDesc = "Perform an HTTP GET request to a given URL"
	}
	if o.postName == "" {
		o.postName = "http_post"
	}
	if o.postDesc == "" {
		o.postDesc = "Perform an HTTP POST request with JSON body to a given URL"
	}
}

// WithHTTPClient sets the HTTP client (e.g. for custom DialContext to mitigate DNS rebinding).
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) {
		o.httpClient = c
	}
}

// WithAllowedDomains sets the whitelist of allowed hostnames. Required for requests to succeed.
// Use exact match (e.g. "api.example.com") or prefix with "." for subdomains (e.g. ".example.com" allows api.example.com).
func WithAllowedDomains(domains []string) Option {
	return func(o *options) {
		o.allowedDomains = domains
	}
}

// WithHeaders sets extra headers to send with every request.
func WithHeaders(h map[string]string) Option {
	return func(o *options) {
		o.headers = h
	}
}

func hasForbiddenHeaders(headers map[string]string) bool {
	for k := range headers {
		switch strings.ToLower(k) {
		case "authorization", "proxy-authorization":
			return true
		}
	}
	return false
}

// WithMaxResponseBody sets the maximum response body size in bytes (default 512KB). Truncated body gets "[Truncated]" suffix.
func WithMaxResponseBody(n int) Option {
	return func(o *options) {
		o.maxResponseBody = n
	}
}

// WithGetName sets the name of the GET tool.
func WithGetName(name string) Option {
	return func(o *options) {
		o.getName = name
	}
}

// WithGetDescription sets the description of the GET tool.
func WithGetDescription(desc string) Option {
	return func(o *options) {
		o.getDesc = desc
	}
}

// WithPostName sets the name of the POST tool.
func WithPostName(name string) Option {
	return func(o *options) {
		o.postName = name
	}
}

// WithPostDescription sets the description of the POST tool.
func WithPostDescription(desc string) Option {
	return func(o *options) {
		o.postDesc = desc
	}
}

// WithAllowPrivateIPs allows requests to private IP ranges. For testing only (e.g. httptest on 127.0.0.1).
func WithAllowPrivateIPs(allow bool) Option {
	return func(o *options) {
		o.allowPrivateIPs = allow
	}
}
