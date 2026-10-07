package release

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestReleaseBootstrapDrainsArchivePadding(t *testing.T) {
	// Arrange: valid trailing archive padding must not SIGPIPE the producer.
	source, remote := releaseFixture(t)
	before := snapshotSource(t, source)
	script := releaseScript(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	shim := `#!/bin/bash
set -euo pipefail
archive=false
output=
for arg in "$@"; do
    case "$arg" in
        archive) archive=true ;;
        --output=*) output=${arg#--output=} ;;
    esac
done
` + strconv.Quote(realGit) + ` "$@"
if [ "$archive" = true ]; then
    if [ -n "$output" ]; then
        dd if=/dev/zero bs=1048576 count=16 >> "$output" 2>/dev/null
    else
        dd if=/dev/zero bs=1048576 count=16 2>/dev/null
    fi
fi
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, "-prepare-only", "patch", ". ./sub")
	cmd.Dir = source
	cmd.Env = append(
		commandEnvironment(false),
		"GIT_CONFIG_COUNT=0",
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	cmd.Stdin = bytes.NewReader(nil)
	// Act: bootstrap and verify actual disposable consumer artifacts, no publish.
	output, runErr := cmd.CombinedOutput()
	// Assert.
	if runErr != nil {
		t.Fatalf("bootstrap padded archive: %v\n%s", runErr, output)
	}
	assertSourceUnchanged(t, source, before)
	if tags := fixtureGit(t, source, "--git-dir", remote, "tag", "-l", "v0.1.1"); tags != "" {
		t.Fatalf("prepare-only published %s", tags)
	}
}
