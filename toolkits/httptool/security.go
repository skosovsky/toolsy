package httptool

import (
	"net"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
)

// validateURL parses rawURL, checks scheme (http/https), allowedDomains, and optionally
// that the resolved host does not point to a private IP (SSRF defense-in-depth).
// If allowPrivateIPs is true (e.g. for tests with httptest on 127.0.0.1), private IP check is skipped.
func validateURL(rawURL string, allowedDomains []string, allowPrivateIPs bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, &toolsy.ClientError{Reason: "invalid URL: " + err.Error(), Err: toolsy.ErrValidation}
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, &toolsy.ClientError{Reason: "only http and https schemes are allowed", Err: toolsy.ErrValidation}
	}

	if len(allowedDomains) == 0 {
		return nil, &toolsy.ClientError{Reason: "no allowed domains configured", Err: toolsy.ErrValidation}
	}

	hostname := strings.TrimSpace(u.Hostname())
	if hostname == "" {
		return nil, &toolsy.ClientError{Reason: "missing host in URL", Err: toolsy.ErrValidation}
	}

	hostLower := strings.ToLower(hostname)
	allowed := false
	for _, d := range allowedDomains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		dLower := strings.ToLower(d)
		if strings.HasPrefix(dLower, ".") {
			// Wildcard: ".slack.com" matches api.slack.com, hooks.slack.com, but not slack.com
			if hostLower != dLower[1:] && strings.HasSuffix(hostLower, dLower) {
				allowed = true
				break
			}
		} else {
			if hostLower == dLower {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return nil, &toolsy.ClientError{Reason: "SSRF: domain not allowed", Err: toolsy.ErrValidation}
	}

	if !allowPrivateIPs {
		ips, err := net.LookupHost(hostname)
		if err != nil {
			return nil, &toolsy.ClientError{Reason: "SSRF: host lookup failed: " + err.Error(), Err: toolsy.ErrValidation}
		}
		for _, ipStr := range ips {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			if isPrivateIP(ip) {
				return nil, &toolsy.ClientError{Reason: "SSRF: private IP resolved", Err: toolsy.ErrValidation}
			}
		}
	}

	return u, nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}
