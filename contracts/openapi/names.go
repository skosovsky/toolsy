package openapi

import (
	"fmt"
	"regexp"
	"strings"
)

// toolNameRegex restricts to LLM provider convention: [a-zA-Z0-9_-]{1,64}
var toolNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// sanitizeToolName converts a string to a valid tool name: lowercase, replace invalid chars with underscore, trim to 64.
func sanitizeToolName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "op"
	}
	s = strings.ToLower(s)
	s = toolNameRegex.ReplaceAllString(s, "_")
	// Collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	if s == "" {
		return "op"
	}
	return s
}

// toolNameFromOperation prefers operationId (sanitized), else method_path (e.g. get_users_id).
// Used names are passed to avoid collisions; on collision append _2, _3, ...
func toolNameFromOperation(operationID, method, path string, used map[string]bool) string {
	base := operationID
	if base == "" {
		base = method + "_" + pathToName(path)
	}
	base = sanitizeToolName(base)
	name := base
	i := 2
	for used[name] {
		name = base + "_" + fmt.Sprintf("%d", i)
		i++
	}
	used[name] = true
	return name
}

// pathToName converts /api/v1/users/{id} to api_v1_users_id
func pathToName(path string) string {
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	return sanitizeToolName(path)
}
