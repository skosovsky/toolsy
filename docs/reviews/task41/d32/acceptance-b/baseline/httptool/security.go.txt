package httptool

import (
	"context"
	"net"
	"net/url"

	"github.com/skosovsky/toolsy"
)

// validateURL parses rawURL, checks scheme (http/https), allowedDomains, and optionally
// that the resolved host does not point to a blocked IP (SSRF defense-in-depth).
// If allowPrivateIPs is true (e.g. for tests with httptest on 127.0.0.1), IP check is skipped.
func validateURL(ctx context.Context, rawURL string, allowedDomains []string, allowPrivateIPs bool) (*url.URL, error) {
	u, hostLower, err := parseHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}

	if len(allowedDomains) == 0 {
		return nil, toolsy.NewValidationError("no allowed domains configured")
	}

	if !HostMatchesAllowedDomains(hostLower, allowedDomains) {
		return nil, toolsy.NewValidationError("SSRF: domain not allowed")
	}

	if !allowPrivateIPs {
		addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
		if lookupErr != nil {
			return nil, toolsy.NewValidationError("SSRF: host lookup failed: " + lookupErr.Error())
		}
		if ipErr := ValidateResolvedIPs(addrs, false); ipErr != nil {
			return nil, ipErr
		}
	}

	return u, nil
}
