package grpc

import (
	"fmt"
	"regexp"
	"strings"
)

var toolNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeToolName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "rpc"
	}
	s = strings.ToLower(s)
	s = toolNameRegex.ReplaceAllString(s, "_")
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	if s == "" {
		return "rpc"
	}
	return s
}

// methodToolName returns ServiceName_MethodName sanitized; used avoids collisions.
func methodToolName(service, method string, used map[string]bool) string {
	base := sanitizeToolName(service) + "_" + sanitizeToolName(method)
	name := base
	i := 2
	for used[name] {
		name = base + "_" + fmt.Sprintf("%d", i)
		i++
	}
	used[name] = true
	return name
}
