package httptool

import (
	"net"
	"strings"

	"github.com/skosovsky/toolsy"
)

// MatchHost compares a bare exact hostname or a leading-dot descendant suffix.
// A suffix excludes its apex; use both entries for apex plus descendants. Case,
// outer whitespace and one terminal DNS root dot are normalized on both sides.
func MatchHost(hostLower, entry string) bool {
	entry = normalizeHostname(entry)
	hostLower = normalizeHostname(hostLower)
	if entry == "" || invalidPolicyHost(hostLower) || strings.HasSuffix(entry, ".") ||
		strings.Contains(entry, "..") {
		return false
	}
	if strings.HasPrefix(entry, ".") {
		base := entry[1:]
		return base != "" && hostLower != base && strings.HasSuffix(hostLower, entry)
	}
	return hostLower == entry
}

func normalizeHostname(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if strings.HasSuffix(host, "..") {
		return host // Leave invalid repeated root dots for rejection, making normalization idempotent.
	}
	return strings.TrimSuffix(host, ".")
}

func invalidPolicyHost(host string) bool {
	return host == "" || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") ||
		strings.Contains(host, "..")
}

// HostBlocked reports whether hostLower is blocked by any entry in blocked (blacklist mode).
func HostBlocked(hostLower string, blocked []string) bool {
	return hostInList(hostLower, blocked)
}

// ValidateResolvedIPs rejects blocked IPs unless allowPrivate is true.
func ValidateResolvedIPs(addrs []net.IPAddr, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	for i := range addrs {
		if IsBlockedIP(addrs[i].IP) {
			return toolsy.NewValidationError("SSRF: private or loopback IP not allowed")
		}
	}
	return nil
}

// HostMatchesAllowedDomains reports whether hostLower is allowed by allowedDomains (whitelist).
func HostMatchesAllowedDomains(hostLower string, allowedDomains []string) bool {
	return hostInList(hostLower, allowedDomains)
}

func hostInList(hostLower string, list []string) bool {
	for _, entry := range list {
		if MatchHost(hostLower, entry) {
			return true
		}
	}
	return false
}
