package openapi

import "net/http"

const defaultMaxResponseBytes = 512 * 1024

// Options configures the OpenAPI parser and executor.
type Options struct {
	HTTPClient       *http.Client
	BaseURL          string
	AuthHeader       string
	AllowedTags      []string
	AllowedMethods   []string
	MaxResponseBytes int
}

func (o *Options) httpClient() *http.Client {
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
