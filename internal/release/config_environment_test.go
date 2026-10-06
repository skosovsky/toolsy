package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateConfigurationPreservesAuthAndDropsSelectors(t *testing.T) {
	// Arrange.
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "core.worktree")
	t.Setenv("GIT_CONFIG_VALUE_0", "/FAKE_OUTSIDE_WORKTREE")
	t.Setenv("GIT_CONFIG_KEY_1", "http.extraHeader")
	t.Setenv("GIT_CONFIG_VALUE_1", "X-Fixture: FAKE_AUTH")
	// Act.
	env := commandEnvironment(true)
	// Assert.
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_COUNT=1") ||
		!strings.Contains(joined, "GIT_CONFIG_KEY_0=http.extraHeader") ||
		!strings.Contains(joined, "GIT_CONFIG_VALUE_0=X-Fixture: FAKE_AUTH") {
		t.Fatal("counted authentication was not preserved")
	}
	if strings.Contains(joined, "FAKE_OUTSIDE_WORKTREE") {
		t.Fatal("repository selector escaped environment guard")
	}
}

func TestSourceCRLFCheckoutIsPreserved(t *testing.T) {
	// Arrange: private checkout normalizes LF, source keeps its own Git policy.
	source, _ := privateFixture(t)
	fixtureGit(t, source, "config", "core.autocrlf", "true")
	value := filepath.Join(source, "value.go")
	if err := os.Remove(value); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, source, "checkout", "--force", "HEAD", "--", "value.go")
	data, err := os.ReadFile(value)
	if err != nil || !strings.Contains(string(data), "\r\n") {
		t.Fatalf("fixture is not CRLF: %v", err)
	}
	if status := fixtureGit(t, source, "status", "--porcelain", "--untracked-files=no"); status != "" {
		t.Fatalf("fixture is not clean: %s", status)
	}
	before := snapshotSource(t, source)
	cfg := fixtureConfig(source)
	cfg.PrepareOnly = true
	// Act.
	err = Run(context.Background(), cfg)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	assertSourceUnchanged(t, source, before)
}
