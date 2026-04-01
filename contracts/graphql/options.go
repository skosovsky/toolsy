package graphql

import "net/http"

const defaultMaxResponseBytes = 512 * 1024

// HTTPClient is the minimal HTTP surface used by GraphQL tools. [*http.Client] and [http.DefaultClient] satisfy it.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options configures the GraphQL introspector and executor.
type Options struct {
	HTTPClient              HTTPClient
	IntrospectionAuthHeader string
	Operations              []string // e.g. []string{"query"} or []string{"query","mutation"}
	MaxResponseBytes        int
}

func (o *Options) httpClient() HTTPClient {
	if o != nil && o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

func (o *Options) maxResponseBytes() int {
	if o != nil && o.MaxResponseBytes > 0 {
		return o.MaxResponseBytes
	}
	return defaultMaxResponseBytes
}
