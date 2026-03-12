package web

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
)

const maxRedirects = 10

// validateScrapeURL checks scheme (http/https), host, blocks private IPs unless allowPrivateIPs,
// and rejects blockedDomains. Returns parsed URL for use in request.
func validateScrapeURL(rawURL string, allowPrivateIPs bool, blockedDomains []string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, &toolsy.ClientError{Reason: "invalid URL: " + err.Error(), Err: toolsy.ErrValidation}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, &toolsy.ClientError{Reason: "only http and https schemes are allowed", Err: toolsy.ErrValidation}
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, &toolsy.ClientError{Reason: "URL host is missing", Err: toolsy.ErrValidation}
	}
	hostLower := strings.ToLower(host)
	for _, b := range blockedDomains {
		if b == "" {
			continue
		}
		blockLower := strings.TrimSpace(strings.ToLower(b))
		if blockLower == hostLower || (len(hostLower) > len(blockLower) && strings.HasSuffix(hostLower, "."+blockLower)) {
			return nil, &toolsy.ClientError{Reason: "SSRF: domain is blocked", Err: toolsy.ErrValidation}
		}
	}
	if !allowPrivateIPs {
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, &toolsy.ClientError{Reason: "SSRF: host lookup failed: " + err.Error(), Err: toolsy.ErrValidation}
		}
		if slices.ContainsFunc(ips, isBlockedIP) {
			return nil, &toolsy.ClientError{Reason: "SSRF: private or loopback IP not allowed", Err: toolsy.ErrValidation}
		}
	}
	return u, nil
}

// isBlockedIP returns true for private, loopback, link-local, unspecified (0.0.0.0, ::), and multicast.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	if ip.Equal(net.IPv4zero) || ip.Equal(net.IPv6zero) {
		return true
	}
	return false
}

// checkRedirect denies redirects to private/loopback IPs and blocked domains (SSRF).
// blockedDomains must be passed so redirect targets are validated against the same blacklist.
func checkRedirect(allowPrivateIPs bool, blockedDomains []string) func(*http.Request, []*http.Request) error {
	return func(redirectReq *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return &toolsy.ClientError{Reason: "too many redirects", Err: toolsy.ErrValidation}
		}
		if _, err := validateScrapeURL(redirectReq.URL.String(), allowPrivateIPs, blockedDomains); err != nil {
			return err
		}
		return nil
	}
}

// rebindingSafeDialContext returns a net.Dialer.Control-style pin: resolve host once, reject private IPs, dial only to resolved IP.
// Used in http.Transport to prevent DNS rebinding (same hostname resolving to private IP after first lookup).
func rebindingSafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, &toolsy.ClientError{Reason: "SSRF: no address for host", Err: toolsy.ErrValidation}
	}
	for i := range ips {
		if isBlockedIP(ips[i].IP) {
			return nil, &toolsy.ClientError{Reason: "SSRF: private or loopback IP not allowed", Err: toolsy.ErrValidation}
		}
	}
	// Pin to first resolved IP so re-lookup cannot switch to private
	dialAddr := net.JoinHostPort(ips[0].IP.String(), port)
	d := net.Dialer{}
	return d.DialContext(ctx, network, dialAddr)
}
