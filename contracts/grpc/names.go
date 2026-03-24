package grpc

import (
	"regexp"
	"strconv"
	"strings"
)

var toolNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

const maxSanitizedToolNameLen = 64

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
	if len(s) > maxSanitizedToolNameLen {
		s = s[:maxSanitizedToolNameLen]
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
		name = base + "_" + strconv.Itoa(i)
		i++
	}
	used[name] = true
	return name
}
