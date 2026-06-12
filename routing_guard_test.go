package toolsy

import (
	"os"
	"strings"
	"testing"
)

func TestCoreTests_NoErrorSubstringRouting(t *testing.T) {
	t.Parallel()
	routingGuardFiles := []string{
		"runpolicy_test.go",
		"session_outcome_test.go",
		"session_state_test.go",
	}
	for _, name := range routingGuardFiles {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			body := string(data)
			if strings.Contains(body, "assert.Contains(t, err.Error(") ||
				strings.Contains(body, "require.Contains(t, err.Error(") {
				t.Fatalf("%s must route errors via AsToolError/requireToolErrorCode, not err.Error() substrings", name)
			}
		})
	}
}
