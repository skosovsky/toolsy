package httptool

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
)

const maxRedirects = 10

// NewSafeHTTPClient returns an [*http.Client] with [SafeDialTransport] and optional redirect validation.
// If redirect is nil, redirects are not allowed.
func NewSafeHTTPClient(opts SafeDialOptions, redirect func(*http.Request, []*http.Request) error) *http.Client {
	client := &http.Client{
		Transport: SafeDialTransport(opts),
	}
	if redirect != nil {
		client.CheckRedirect = redirect
	} else {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

// MergeHTTPClient applies timeout from base onto a safe client. Custom Transport on base is ignored.
func MergeHTTPClient(safe *http.Client, base HTTPClient) *http.Client {
	if base == nil {
		return safe
	}
	std, ok := base.(*http.Client)
	if !ok || std == nil {
		return safe
	}
	if std.Timeout <= 0 {
		return safe
	}
	merged := *safe
	merged.Timeout = std.Timeout
	return &merged
}

func defaultHTTPClient(o *options) *http.Client {
	opts := SafeDialOptions{ //nolint:exhaustruct // whitelist mode; IP policy defaults
		AllowedHosts:    o.allowedDomains,
		AllowPrivateIPs: o.allowPrivateIPs,
	}
	safe := NewSafeHTTPClient(opts, CheckRedirectAllowed(o.allowedDomains, o.allowPrivateIPs))
	if o.httpClient == nil {
		return safe
	}
	return MergeHTTPClient(safe, o.httpClient)
}

// CheckRedirectAllowed validates redirect URLs against allowedDomains whitelist (httptool tool mode).
func CheckRedirectAllowed(allowedDomains []string, allowPrivateIPs bool) func(*http.Request, []*http.Request) error {
	return func(redirectReq *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return toolsy.NewValidationError("too many redirects")
		}
		_, err := validateURL(
			redirectReq.Context(),
			redirectReq.URL.String(),
			allowedDomains,
			allowPrivateIPs,
		)
		return err
	}
}

// CheckRedirectRemote validates redirect URLs via [ValidateRemoteURL] and optional host blacklist.
func CheckRedirectRemote(allowPrivateIPs bool, blockedHosts []string) func(*http.Request, []*http.Request) error {
	return func(redirectReq *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return toolsy.NewValidationError("too many redirects")
		}
		if err := ValidateRemoteURL(redirectReq.Context(), redirectReq.URL.String(), allowPrivateIPs); err != nil {
			return err
		}
		hostLower := strings.ToLower(strings.TrimSpace(redirectReq.URL.Hostname()))
		if HostBlocked(hostLower, blockedHosts) {
			return toolsy.NewValidationError("SSRF: domain is blocked")
		}
		return nil
	}
}

// ValidateRemoteURL checks scheme, host, and resolved IPs for URL-level SSRF (blacklist mode, no host whitelist).
func ValidateRemoteURL(ctx context.Context, rawURL string, allowPrivateIPs bool) error {
	u, _, err := parseHTTPURL(rawURL)
	if err != nil {
		return err
	}
	return validateRemoteURLParsed(ctx, u, allowPrivateIPs)
}

func validateRemoteURLParsed(ctx context.Context, u *url.URL, allowPrivateIPs bool) error {
	if allowPrivateIPs {
		return nil
	}
	addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if lookupErr != nil {
		return toolsy.NewValidationError("SSRF: host lookup failed: " + lookupErr.Error())
	}
	return ValidateResolvedIPs(addrs, false)
}

// ValidateRemoteURLWithBlacklist checks [ValidateRemoteURL] and host blacklist entries.
func ValidateRemoteURLWithBlacklist(
	ctx context.Context,
	rawURL string,
	allowPrivateIPs bool,
	blockedHosts []string,
) (*url.URL, error) {
	u, hostLower, err := parseHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := validateRemoteURLParsed(ctx, u, allowPrivateIPs); err != nil {
		return nil, err
	}
	if HostBlocked(hostLower, blockedHosts) {
		return nil, toolsy.NewValidationError("SSRF: domain is blocked")
	}
	return u, nil
}
