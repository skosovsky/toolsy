package httptool

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/skosovsky/toolsy"
)

const defaultDialTimeout = 30 * time.Second

// IsPrivateIP reports whether ip is loopback, link-local unicast, or private (RFC1918, etc.).
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}

// IsBlockedIP reports whether ip must be blocked for SSRF-safe dialing (private, loopback,
// link-local, unspecified, multicast).
func IsBlockedIP(ip net.IP) bool {
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

// SafeDialOptions configures [SafeDialTransport] host and IP filtering.
//
// Host policy (fail-closed):
//   - When AllowedHosts is non-empty (strict whitelist): only listed hosts are permitted.
//     Every matching BlockedHosts entry takes precedence, including suffix overlap.
//     A configured nonempty allowlist with only blank entries permits no hosts.
//   - When AllowedHosts is empty (blacklist mode): hosts matching BlockedHosts are denied.
//
// IP policy: IsBlockedIP is always applied at dial time unless AllowPrivateIPs is true.
// Both lists use MatchHost syntax: bare exact hostname, leading dot descendants
// only; apex plus descendants requires both entries.
type SafeDialOptions struct {
	BlockedHosts    []string
	AllowedHosts    []string
	IsBlockedIP     func(net.IP) bool
	DialTimeout     time.Duration
	AllowPrivateIPs bool
}

// SafeDialTransport returns an [*http.Transport] with SSRF-safe dialing.
// At dial time it resolves the host, checks each IP with IsBlockedIP (unless AllowPrivateIPs),
// and connects to the first resolved address (DNS-rebinding pin). URL-level checks in
// ValidateRemoteURL use the same IP policy at validate time before the request is sent.
func SafeDialTransport(opts SafeDialOptions) *http.Transport {
	isBlocked := opts.IsBlockedIP
	if isBlocked == nil {
		isBlocked = IsBlockedIP
	}
	timeout := opts.DialTimeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	policy := normalizeHostPolicy(opts.AllowedHosts, opts.BlockedHosts)
	return &http.Transport{
		DialContext: safeDialContext(isBlocked, opts.AllowPrivateIPs, timeout, policy),
	}
}

type hostPolicy struct {
	whitelist bool
	allowed   []string
	blocked   []string
}

func normalizeHostPolicy(allowed, blocked []string) hostPolicy {
	return hostPolicy{
		whitelist: len(allowed) > 0,
		allowed:   normalizeHostList(allowed),
		blocked:   normalizeHostList(blocked),
	}
}

func normalizeHostList(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = normalizeHostname(h)
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

func hostAllowed(host string, policy hostPolicy) bool {
	host = normalizeHostname(host)
	if invalidPolicyHost(host) {
		return false
	}
	if hostInList(host, policy.blocked) {
		return false
	}
	return !policy.whitelist || hostInList(host, policy.allowed)
}

func safeDialContext(
	isBlocked func(net.IP) bool,
	allowPrivateIPs bool,
	timeout time.Duration,
	policy hostPolicy,
) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if !hostAllowed(host, policy) {
			return nil, toolsy.NewValidationError("SSRF: host not allowed")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, toolsy.NewValidationError("SSRF: no address for host")
		}
		for i := range ips {
			if !allowPrivateIPs && isBlocked(ips[i].IP) {
				return nil, toolsy.NewValidationError("SSRF: private or loopback IP not allowed")
			}
		}
		dialAddr := net.JoinHostPort(ips[0].IP.String(), port)
		d := net.Dialer{Timeout: timeout} //nolint:exhaustruct_v5 // defaults for DNS pin dial
		return d.DialContext(ctx, network, dialAddr)
	}
}
