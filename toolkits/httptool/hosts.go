package httptool

import (
	"net"
	"strings"

	"github.com/skosovsky/toolsy"
)

// MatchHost reports whether hostLower matches entry (exact, subdomain, or ".suffix" wildcard).
func MatchHost(hostLower, entry string) bool {
	entry = strings.TrimSpace(strings.ToLower(entry))
	if entry == "" {
		return false
	}
	hostLower = strings.TrimSpace(strings.ToLower(hostLower))
	if hostLower == entry {
		return true
	}
	if strings.HasPrefix(entry, ".") {
		base := entry[1:]
		return hostLower != base && strings.HasSuffix(hostLower, entry)
	}
	return len(hostLower) > len(entry) && strings.HasSuffix(hostLower, "."+entry)
}

// HostBlocked reports whether hostLower is blocked by any entry in blocked (blacklist mode).
func HostBlocked(hostLower string, blocked []string) bool {
	hostLower = strings.TrimSpace(strings.ToLower(hostLower))
	for _, b := range blocked {
		if MatchHost(hostLower, b) {
			return true
		}
	}
	return false
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
	for _, d := range allowedDomains {
		if MatchHost(hostLower, d) {
			return true
		}
	}
	return false
}

func hostInList(hostLower string, list []string) bool {
	for _, entry := range list {
		if MatchHost(hostLower, entry) {
			return true
		}
	}
	return false
}
