package openapi

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// toolNameRegex restricts to LLM provider convention: [a-zA-Z0-9_-]{1,64}.
var toolNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

const maxSanitizedToolNameLen = 64

// sanitizeToolName converts a string to a valid tool name: lowercase, replace invalid chars with underscore, trim to max length.
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
	if len(s) > maxSanitizedToolNameLen {
		s = s[:maxSanitizedToolNameLen]
	}
	if s == "" {
		return "op"
	}
	return s
}

// toolNameFromOperation prefers operationId (sanitized), else method_path (e.g. get_users_id).
// Sorted discovery order and an operation identity hash make collision names stable.
func toolNameFromOperation(operationID, method, path string, used map[string]bool) string {
	base := operationID
	if base == "" {
		base = method + "_" + pathToName(path)
	}
	base = sanitizeToolName(base)
	name := base
	if used[name] {
		identity := sha256.Sum256([]byte(method + " " + path))
		suffix := fmt.Sprintf("_%x", identity[:6])
		name = base[:min(len(base), maxSanitizedToolNameLen-len(suffix))] + suffix
		for attempt := 2; used[name]; attempt++ {
			candidateSuffix := fmt.Sprintf("%s_%d", suffix, attempt)
			name = base[:min(len(base), maxSanitizedToolNameLen-len(candidateSuffix))] + candidateSuffix
		}
	}
	used[name] = true
	return name
}

// pathToName converts /api/v1/users/{id} to api_v1_users_id.
func pathToName(path string) string {
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	return sanitizeToolName(path)
}
