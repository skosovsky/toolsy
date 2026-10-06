package httptool

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// HTTPClient is the minimal HTTP surface used by httptool. Pass [*http.Client] with Timeout only;
// Transport is always merged from the default SSRF-safe client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Option configures AsTools (client, allowed domains, headers, limits, names).
type Option func(*options)

type options struct {
	httpClient        HTTPClient
	allowedDomains    []string
	headers           map[string]string
	maxResponseBody   int
	maxWireBytes      int
	maxRequestBody    int
	credentialOrigins []string
	getName           string
	getDesc           string
	postName          string
	postDesc          string
	allowPrivateIPs   bool // internal use for tests only
}

const (
	defaultMaxResponseBody = 512 * 1024
	defaultMaxWireBytes    = 4 * 1024 * 1024
	maxURLBytes            = 8192
	defaultTimeout         = 30 * time.Second
)

func applyDefaults(o *options) {
	if o.maxRequestBody <= 0 {
		o.maxRequestBody = defaultMaxResponseBody
	}
	if o.maxResponseBody <= 0 {
		o.maxResponseBody = defaultMaxResponseBody
	}
	if o.maxWireBytes <= 0 {
		o.maxWireBytes = defaultMaxWireBytes
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

// WithHTTPClient sets a custom [http.Client]. Only Timeout is merged onto the default SafeDialTransport
// client; Transport and CheckRedirect from the custom client are ignored for SSRF safety.
func WithHTTPClient(c HTTPClient) Option {
	return func(o *options) {
		o.httpClient = c
	}
}

// WithAllowedDomains sets the whitelist of allowed hostnames. Required for requests to succeed.
// Use exact match (e.g. "api.example.com") or prefix with "." for subdomains (e.g. ".example.com" allows api.example.com).
func WithAllowedDomains(domains []string) Option {
	return func(o *options) {
		o.allowedDomains = slices.Clone(domains)
	}
}

// WithHeaders sets non-secret metadata headers. Authentication headers are rejected;
// use WithCredentialOrigins and the run credentials provider for authentication.
func WithHeaders(h map[string]string) Option {
	return func(o *options) {
		o.headers = maps.Clone(h)
	}
}

func hasForbiddenHeaders(headers map[string]string) bool {
	for k := range headers {
		switch strings.ToLower(k) {
		case "authorization", "proxy-authorization", "cookie":
			return true
		}
	}
	return false
}

// WithMaxResponseBody sets the maximum response body field size in bytes for probe GET/POST (default 512KB).
// This is the budget for the "body" value inside {"status":N,"body":"..."}, not the full wire JSON envelope.
// Library ReadBodyLimited returns textprocessor.ErrReadLimitExceeded on exceed; probe tools return CodeValidationFailed.
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

// WithMaxWireBytes bounds the complete encoded JSON result. Nonpositive uses 4 MiB.
func WithMaxWireBytes(n int) Option { return func(o *options) { o.maxWireBytes = n } }

// WithCredentialOrigins explicitly authorizes run credentials for exact scheme/host/port origins.
// Unlisted origins receive no credentials even when their domain is allowed.
func WithCredentialOrigins(origins []string) Option {
	return func(o *options) { o.credentialOrigins = append([]string(nil), origins...) }
}

func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func normalizeCredentialOrigins(o *options) error {
	for i, raw := range o.credentialOrigins {
		u, _, err := parseHTTPURL(raw)
		if err != nil {
			return fmt.Errorf("credential origin: %w", err)
		}
		if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("credential binding must be an origin: %s", raw)
		}
		o.credentialOrigins[i] = origin(u)
	}
	return nil
}

// WithMaxRequestBody rejects oversize POST input before dispatch without modifying it.
// Nonpositive uses 512 KiB.
func WithMaxRequestBody(n int) Option { return func(o *options) { o.maxRequestBody = n } }
