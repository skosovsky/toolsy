package graphql

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
		return "op"
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
		return "op"
	}
	return s
}

// toolName returns graphql_<operationName> sanitized; usedNames avoids collisions.
func toolName(operationName string, usedNames map[string]bool) string {
	base := "graphql_" + sanitizeToolName(operationName)
	name := base
	i := 2
	for usedNames[name] {
		name = base + "_" + strconv.Itoa(i)
		i++
	}
	usedNames[name] = true
	return name
}
