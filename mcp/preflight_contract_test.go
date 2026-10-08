package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProductionProtocolHasNoLegacyFallback(t *testing.T) {
	// Arrange.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := regexp.MustCompile(
		`2025-11-25|2025-06-18|2024-11-05|InitializeParams|InitializeResult|MethodInitialize|MethodInitialized|ErrSessionExpired|WithClientRoots|WithRoots|OnRequest\s*\(|Mcp-Session-Id|Last-Event-ID|http\.MethodGet|http\.MethodDelete`,
	)
	methods := regexp.MustCompile(
		`"initialize"|"notifications/initialized"|"roots/list"|"logging/setLevel"|"resources/(subscribe|unsubscribe)"|"ping"`,
	)
	// Act / Assert: transport.go retains the fail-closed method denylist.
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if match := forbidden.Find(source); match != nil {
			t.Errorf("%s: legacy protocol contract %s", file, match)
		}
		if file != "transport.go" {
			if match := methods.Find(source); match != nil {
				t.Errorf("%s: legacy method %s", file, match)
			}
		}
	}
	if ProtocolVersion != "2026-07-28" {
		t.Fatalf("unexpected protocol version: %s", ProtocolVersion)
	}
}
