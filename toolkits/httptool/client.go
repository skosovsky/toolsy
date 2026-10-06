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

// RedirectError reports a redirect refused after the original request was sent.
// It never authorizes argument correction or retry: the original request may have
// produced effects. Reason contains policy text, never request URLs or credentials.
type RedirectError struct {
	Reason string
	cause  error
}

func (e *RedirectError) Error() string { return "HTTP redirect refused: " + e.Reason }

// Unwrap preserves the destination validation cause when present.
func (e *RedirectError) Unwrap() error { return e.cause }

func refusedRedirect(reason string, cause error) error {
	return &toolsy.ToolError{
		Code:        toolsy.CodeRemoteExecution,
		Retryable:   false,
		Reason:      "HTTP redirect refused",
		FixableArgs: nil,
		SafeMessage: "HTTP redirect refused",
		Err:         &RedirectError{Reason: reason, cause: cause},
	}
}

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
	opts := SafeDialOptions{ //nolint:exhaustruct_v5 // whitelist mode; IP policy defaults
		AllowedHosts:    o.allowedDomains,
		AllowPrivateIPs: o.allowPrivateIPs,
	}
	safe := NewSafeHTTPClient(opts, CheckRedirectAllowed(o.allowedDomains, o.allowPrivateIPs))
	safe.Timeout = defaultTimeout
	if o.httpClient == nil {
		return safe
	}
	return MergeHTTPClient(safe, o.httpClient)
}

// CheckRedirectAllowed follows only requests that started as GET/HEAD, validating
// destinations against allowedDomains. Foreign origins lose credential headers.
// Other initial methods are refused even when HTTP rewrites the redirect to GET.
func CheckRedirectAllowed(allowedDomains []string, allowPrivateIPs bool) func(*http.Request, []*http.Request) error {
	return func(redirectReq *http.Request, via []*http.Request) error {
		if err := validateRedirectRead(redirectReq, via); err != nil {
			return err
		}
		if origin(redirectReq.URL) != origin(via[0].URL) {
			stripRedirectCredentials(redirectReq.Header)
		}
		_, err := validateURL(
			redirectReq.Context(),
			redirectReq.URL.String(),
			allowedDomains,
			allowPrivateIPs,
		)
		if err != nil {
			return refusedRedirect("destination rejected by URL policy", err)
		}
		return nil
	}
}

// CheckRedirectRemote follows only GET/HEAD requests within the original
// scheme/hostname/effective-port origin, with URL/IP validation and host blacklist.
// All other redirects fail with RedirectError before a second dispatch. Hosts
// must configure the final RPC endpoint explicitly; request bodies are never rerouted.
func CheckRedirectRemote(allowPrivateIPs bool, blockedHosts []string) func(*http.Request, []*http.Request) error {
	return func(redirectReq *http.Request, via []*http.Request) error {
		if err := validateRedirectRead(redirectReq, via); err != nil {
			return err
		}
		if origin(redirectReq.URL) != origin(via[0].URL) {
			return refusedRedirect("destination origin differs from the original request", nil)
		}
		if err := ValidateRemoteURL(redirectReq.Context(), redirectReq.URL.String(), allowPrivateIPs); err != nil {
			return refusedRedirect("destination rejected by URL policy", err)
		}
		hostLower := strings.ToLower(strings.TrimSpace(redirectReq.URL.Hostname()))
		if HostBlocked(hostLower, blockedHosts) {
			return refusedRedirect("destination rejected by host policy", nil)
		}
		return nil
	}
}

func validateRedirectRead(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || req == nil || req.URL == nil || via[0] == nil || via[0].URL == nil {
		return refusedRedirect("missing original or destination request", nil)
	}
	if len(via) >= maxRedirects {
		return refusedRedirect("too many redirects", nil)
	}
	if !redirectReadMethod(via[0].Method) || !redirectReadMethod(req.Method) {
		return refusedRedirect("only requests that started as GET or HEAD may redirect", nil)
	}
	return nil
}

func redirectReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func stripRedirectCredentials(headers http.Header) {
	for name := range headers {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") ||
			strings.EqualFold(name, "Proxy-Authorization") {
			delete(headers, name)
		}
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
