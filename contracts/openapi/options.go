package openapi

import (
	"net/http"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const defaultMaxResponseBytes = 512 * 1024

// Options configures the OpenAPI parser and executor.
type Options struct {
	HTTPSettings     httptool.ClientSettings
	client           *http.Client
	BaseURL          string
	AllowedTags      []string
	AllowedMethods   []string
	MaxResponseBytes int
	// AllowPrivateIPs relaxes SSRF IP blocking for tests and private networks (e.g. httptest on 127.0.0.1).
	AllowPrivateIPs bool
}

func (o *Options) httpClient() (*http.Client, bool, error) {
	if o.client != nil {
		return o.client, false, nil
	}
	client, err := httptool.NewConfiguredSafeHTTPClient(
		httptool.SafeDialOptions{ //nolint:exhaustruct_v5 // Unset fields retain safe defaults.
			AllowPrivateIPs: o.AllowPrivateIPs,
		},
		httptool.CheckRedirectRemote(o.AllowPrivateIPs, nil),
		o.HTTPSettings,
	)
	return client, true, err
}

func (o *Options) maxResponseBytes() int {
	if o != nil && o.MaxResponseBytes > 0 {
		return o.MaxResponseBytes
	}
	return defaultMaxResponseBytes
}
