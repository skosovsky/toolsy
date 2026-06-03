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
func validateScrapeURL(
	ctx context.Context,
	rawURL string,
	allowPrivateIPs bool,
	blockedDomains []string,
) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, toolsy.NewValidationError("invalid URL: " + err.Error())
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, toolsy.NewValidationError("only http and https schemes are allowed")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, toolsy.NewValidationError("URL host is missing")
	}
	hostLower := strings.ToLower(host)
	for _, b := range blockedDomains {
		if b == "" {
			continue
		}
		blockLower := strings.TrimSpace(strings.ToLower(b))
		if blockLower == hostLower ||
			(len(hostLower) > len(blockLower) && strings.HasSuffix(hostLower, "."+blockLower)) {
			return nil, toolsy.NewValidationError("SSRF: domain is blocked")
		}
	}
	if !allowPrivateIPs {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, toolsy.NewValidationError("SSRF: host lookup failed: " + err.Error())
		}
		if slices.ContainsFunc(addrs, func(a net.IPAddr) bool { return isBlockedIP(a.IP) }) {
			return nil, toolsy.NewValidationError("SSRF: private or loopback IP not allowed")
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
			return toolsy.NewValidationError("too many redirects")
		}
		if _, err := validateScrapeURL(
			redirectReq.Context(),
			redirectReq.URL.String(),
			allowPrivateIPs,
			blockedDomains,
		); err != nil {
			return err
		}
		return nil
	}
}

// rebindingSafeDialContext returns a net.Dialer.Control-style pin: resolve host once, reject private IPs, dial only to resolved IP.
// Used in [http.Transport] to prevent DNS rebinding (same hostname resolving to private IP after first lookup).
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
		return nil, toolsy.NewValidationError("SSRF: no address for host")
	}
	for i := range ips {
		if isBlockedIP(ips[i].IP) {
			return nil, toolsy.NewValidationError("SSRF: private or loopback IP not allowed")
		}
	}
	// Pin to first resolved IP so re-lookup cannot switch to private
	dialAddr := net.JoinHostPort(ips[0].IP.String(), port)
	var d net.Dialer
	return d.DialContext(ctx, network, dialAddr)
}
