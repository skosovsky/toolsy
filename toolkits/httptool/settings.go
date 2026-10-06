package httptool

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

// ClientSettings configures an owned SSRF-safe transport. Zero Timeout uses the
// caller's context deadline. TLSConfig is cloned; certificates, roots and callback
// state referenced by it must remain immutable for the configuration lifetime.
// Custom dialers, proxies, Do implementations and redirect callbacks are unsupported.
type ClientSettings struct {
	Timeout   time.Duration
	TLSConfig *tls.Config
}

// NewConfiguredSafeHTTPClient constructs one reusable pool. The caller owns it
// and may call CloseIdleConnections when disposing its configuration.
func NewConfiguredSafeHTTPClient(
	opts SafeDialOptions,
	redirect func(*http.Request, []*http.Request) error,
	settings ClientSettings,
) (*http.Client, error) {
	if settings.Timeout < 0 {
		return nil, errors.New("HTTP timeout must not be negative")
	}
	client := NewSafeHTTPClient(opts, redirect)
	client.Timeout = settings.Timeout
	if settings.TLSConfig != nil {
		transport := client.Transport.(*http.Transport) //nolint:errcheck // NewSafeHTTPClient always constructs this concrete transport.
		transport.TLSClientConfig = settings.TLSConfig.Clone()
	}
	return client, nil
}
