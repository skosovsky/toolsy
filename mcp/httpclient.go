package mcp

import (
	"net/http"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// SSETransportOption configures [NewSSETransport].
type SSETransportOption func(*sseTransportImpl)

// WithSSEAllowPrivateIPs relaxes SSRF IP blocking for SSE GET/POST (tests only).
func WithSSEAllowPrivateIPs(allow bool) SSETransportOption {
	return func(impl *sseTransportImpl) {
		impl.allowPrivateIPs = allow
		impl.client = defaultSSEHTTPClient(allow)
	}
}

// WithSSEMaxStreamBytes sets the total byte budget for the SSE response stream (default 16 MiB).
func WithSSEMaxStreamBytes(n int) SSETransportOption {
	return func(impl *sseTransportImpl) {
		if n > 0 {
			impl.maxStreamBytes = n
		}
	}
}

// WithSSEHTTPClient sets the HTTP client for SSE GET/POST. Only Timeout is merged onto the default
// SSRF-safe client (Timeout 0 for long-lived streams); custom Transport is ignored.
func WithSSEHTTPClient(c *http.Client) SSETransportOption {
	return func(impl *sseTransportImpl) {
		merged := httptool.MergeHTTPClient(defaultSSEHTTPClient(impl.allowPrivateIPs), c)
		if c != nil && c.Timeout <= 0 {
			merged.Timeout = 0
		}
		impl.client = merged
	}
}

func defaultSSEHTTPClient(allowPrivateIPs bool) *http.Client {
	client := httptool.NewSafeHTTPClient(
		httptool.SafeDialOptions{AllowPrivateIPs: allowPrivateIPs}, //nolint:exhaustruct // blacklist when false
		httptool.CheckRedirectRemote(allowPrivateIPs, nil),
	)
	client.Timeout = 0 // long-lived SSE stream; per-call timeouts at transport layer
	return client
}
