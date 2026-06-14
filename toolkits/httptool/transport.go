package httptool

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// SafeDialOptions configures SSRF-safe dial behavior for HTTP transports.
type SafeDialOptions struct {
	// BlockedHosts blocks exact hostnames (lowercase) before DNS resolution.
	BlockedHosts map[string]struct{}
	// AllowedHosts skips IP block checks for whitelisted internal services (e.g. SearXNG in Docker).
	AllowedHosts map[string]struct{}
	// IsBlockedIP returns true when a resolved IP must not be connected to.
	// When nil, IsPrivateIP is used.
	IsBlockedIP func(net.IP) bool
	DialTimeout time.Duration
	KeepAlive   time.Duration
}

// SafeDialTransport clones base and installs a DialContext hook that blocks SSRF targets.
func SafeDialTransport(base *http.Transport, opts SafeDialOptions) *http.Transport {
	if base == nil {
		return nil
	}
	tr := base.Clone()
	isBlocked := opts.IsBlockedIP
	if isBlocked == nil {
		isBlocked = IsPrivateIP
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	keepAlive := opts.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 30 * time.Second
	}
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: keepAlive,
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		hostLower := strings.ToLower(host)
		if _, allowed := opts.AllowedHosts[hostLower]; allowed {
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		}
		if _, blocked := opts.BlockedHosts[hostLower]; blocked {
			return nil, fmt.Errorf("connect blocked for host %q (SSRF protection)", host)
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no IP found for host %q", host)
		}
		for _, ipa := range ips {
			if isBlocked(ipa.IP) {
				return nil, fmt.Errorf("connect blocked: host %q resolves to blocked IP %s (SSRF protection)", host, ipa.IP)
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return tr
}

// DefaultLoopbackBlockedHosts returns a host blocklist for localhost-style targets.
func DefaultLoopbackBlockedHosts() map[string]struct{} {
	return map[string]struct{}{
		"localhost": {},
		"127.0.0.1": {},
		"0.0.0.0":   {},
		"::1":       {},
	}
}
