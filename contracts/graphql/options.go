package graphql

import "net/http"

const defaultMaxResponseBytes = 512 * 1024

// HTTPClient is the minimal HTTP surface used by GraphQL tools. Pass [*http.Client] with Timeout only;
// Transport is always merged from the default SSRF-safe client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options configures the GraphQL introspector and executor.
type Options struct {
	HTTPClient              HTTPClient
	IntrospectionAuthHeader string
	Operations              []string // e.g. []string{"query"} or []string{"query","mutation"}
	MaxResponseBytes        int
	// AllowPrivateIPs relaxes SSRF IP blocking for tests and private networks (e.g. httptest on 127.0.0.1).
	AllowPrivateIPs bool
}

func (o *Options) httpClient() HTTPClient {
	allowPrivate := false
	if o != nil {
		allowPrivate = o.AllowPrivateIPs
		if o.HTTPClient != nil {
			return resolveHTTPClient(o.HTTPClient, allowPrivate)
		}
	}
	return defaultHTTPClient(allowPrivate)
}

func (o *Options) maxResponseBytes() int {
	if o != nil && o.MaxResponseBytes > 0 {
		return o.MaxResponseBytes
	}
	return defaultMaxResponseBytes
}
