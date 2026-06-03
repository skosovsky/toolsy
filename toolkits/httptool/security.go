package httptool

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
)

// validateURL parses rawURL, checks scheme (http/https), allowedDomains, and optionally
// that the resolved host does not point to a private IP (SSRF defense-in-depth).
// If allowPrivateIPs is true (e.g. for tests with httptest on 127.0.0.1), private IP check is skipped.
func validateURL(ctx context.Context, rawURL string, allowedDomains []string, allowPrivateIPs bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, toolsy.NewValidationError("invalid URL: " + err.Error())
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, toolsy.NewValidationError("only http and https schemes are allowed")
	}

	if len(allowedDomains) == 0 {
		return nil, toolsy.NewValidationError("no allowed domains configured")
	}

	hostname := strings.TrimSpace(u.Hostname())
	if hostname == "" {
		return nil, toolsy.NewValidationError("missing host in URL")
	}

	hostLower := strings.ToLower(hostname)
	if !hostMatchesAllowedDomains(hostLower, allowedDomains) {
		return nil, toolsy.NewValidationError("SSRF: domain not allowed")
	}

	if !allowPrivateIPs {
		ips, err := net.DefaultResolver.LookupHost(ctx, hostname)
		if err != nil {
			return nil, toolsy.NewValidationError("SSRF: host lookup failed: " + err.Error())
		}
		for _, ipStr := range ips {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			if isPrivateIP(ip) {
				return nil, toolsy.NewValidationError("SSRF: private IP resolved")
			}
		}
	}

	return u, nil
}

// hostMatchesAllowedDomains reports whether hostLower is allowed by allowedDomains
// (exact match or wildcard entry starting with ".").
func hostMatchesAllowedDomains(hostLower string, allowedDomains []string) bool {
	for _, d := range allowedDomains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		dLower := strings.ToLower(d)
		if strings.HasPrefix(dLower, ".") {
			// Wildcard: ".slack.com" matches api.slack.com, hooks.slack.com, but not slack.com
			if hostLower != dLower[1:] && strings.HasSuffix(hostLower, dLower) {
				return true
			}
		} else {
			if hostLower == dLower {
				return true
			}
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}
