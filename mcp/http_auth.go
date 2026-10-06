package mcp

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxAuthHeaderBytes    = 4096
	maxAuthChallenges     = 4
	maxAuthParameterBytes = 1024
)

// HTTPAuthChallenge is a deliberately narrow, untrusted authentication hint.
// Free-text parameters and token68 credentials are never retained.
// ResourceMetadata is an HTTPS URL hint; only the host may choose to use it.
type HTTPAuthChallenge struct {
	Scheme           string
	ErrorCode        string
	ResourceMetadata string
}

// String and LogValue deliberately omit challenge parameter values.
func (c HTTPAuthChallenge) String() string { return "MCP authentication challenge" }

func (c HTTPAuthChallenge) LogValue() slog.Value {
	return slog.StringValue("MCP authentication challenge")
}

func (c HTTPAuthChallenge) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(c.String())) }

// Format keeps formatting verbs such as %+v and %#v from exposing hints.
func (e HTTPError) Format(state fmt.State, _ rune) { _, _ = state.Write([]byte(e.Error())) }

func (e HTTPError) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("status", e.StatusCode), slog.String("operation", e.Operation))
}

func authenticationHTTPError(response *http.Response) *HTTPError {
	challenges, truncated := parseAuthChallenges(response.Header.Values("WWW-Authenticate"))
	return &HTTPError{
		StatusCode: response.StatusCode, Operation: "POST authentication",
		Challenges: challenges, ChallengesTruncated: truncated,
	}
}

func parseAuthChallenges(headers []string) ([]HTTPAuthChallenge, bool) {
	var challenges []HTTPAuthChallenge
	budget := maxAuthHeaderBytes
	truncated := false
	for _, header := range headers {
		if len(header)+1 > budget {
			truncated = true
			break
		}
		budget -= len(header) + 1
		parsed, exceeded := parseAuthHeader(header, maxAuthChallenges-len(challenges))
		challenges = append(challenges, parsed...)
		if exceeded {
			return challenges, true
		}
	}
	return challenges, truncated
}

func parseAuthHeader(header string, limit int) ([]HTTPAuthChallenge, bool) {
	segments, valid := authSegments(header)
	if !valid {
		return nil, false
	}
	var challenges []HTTPAuthChallenge
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		key, value, parameter := strings.Cut(segment, "=")
		if parameter && validHTTPToken(strings.TrimSpace(key)) {
			if len(challenges) > 0 {
				applyAuthParameter(&challenges[len(challenges)-1], strings.TrimSpace(key), value)
			}
			continue
		}
		scheme, rest, found := strings.Cut(segment, " ")
		if !validHTTPToken(scheme) || len(scheme) > 32 {
			return nil, false
		}
		if len(challenges) == limit {
			return challenges, true
		}
		challenges = append(challenges, HTTPAuthChallenge{Scheme: scheme, ErrorCode: "", ResourceMetadata: ""})
		if found {
			key, value, parameter = strings.Cut(strings.TrimSpace(rest), "=")
			if parameter {
				applyAuthParameter(&challenges[len(challenges)-1], strings.TrimSpace(key), value)
			}
		}
	}
	return challenges, false
}

func authSegments(header string) ([]string, bool) {
	var segments []string
	quoted, escaped, start := false, false, 0
	for index := range len(header) {
		char := header[index]
		if char < 32 && char != '\t' || char == 127 {
			return nil, false
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
		}
		if char == ',' && !quoted {
			segments = append(segments, strings.TrimSpace(header[start:index]))
			start = index + 1
		}
	}
	if quoted || escaped {
		return nil, false
	}
	return append(segments, strings.TrimSpace(header[start:])), true
}

func applyAuthParameter(challenge *HTTPAuthChallenge, key, raw string) {
	if !strings.EqualFold(challenge.Scheme, "Bearer") {
		return
	}
	value := strings.TrimSpace(raw)
	if len(value) > maxAuthParameterBytes {
		return
	}
	if strings.HasPrefix(value, "\"") {
		if len(value) < 2 || !strings.HasSuffix(value, "\"") {
			return
		}
		value = value[1 : len(value)-1]
		// Escaped or embedded quotes are not needed for the supported safe subset.
		if strings.ContainsAny(value, "\\\"") {
			return
		}
	} else if !validHTTPToken(value) {
		return
	}
	switch strings.ToLower(key) {
	case "error":
		switch value {
		case "invalid_request", "invalid_token", "insufficient_scope":
			challenge.ErrorCode = value
		}
	case "resource_metadata":
		parsed, err := url.Parse(value)
		if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil &&
			parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && parsed.Opaque == "" {
			challenge.ResourceMetadata = parsed.String()
		}
	}
}
