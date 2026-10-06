package release

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicationScope(t *testing.T) {
	// Arrange: unrelated annotated and lightweight tags exist locally.
	source, remote := releaseFixture(t)
	fixtureGit(t, source, "tag", "private-experiment")
	fixtureGit(t, source, "-c", "tag.gpgSign=false", "tag", "-a", "private-annotated", "-m", "private")
	fixtureGit(t, source, "config", "push.followTags", "true")
	before := snapshotSource(t, source)
	var output bytes.Buffer
	// Act.
	err := Run(
		context.Background(),
		Config{Dir: source, Mode: "patch", Input: io.NopCloser(strings.NewReader("y\n")), Output: &output},
	)
	// Assert.
	if err != nil {
		t.Fatalf("release: %v\n%s", err, &output)
	}
	if tags := fixtureGit(t, remote, "tag", "--list"); tags != "sub/v0.1.1\nv0.1.0\nv0.1.1" {
		t.Fatalf("published tags: %q", tags)
	}
	assertSourceUnchanged(t, source, before)
}

func TestPublicationFailures(t *testing.T) {
	for _, scenario := range []string{"local_collision", "push_collision", "reject_child", "no_atomic", "multiple_destinations"} {
		t.Run(scenario, func(t *testing.T) {
			// Arrange: each failure must preserve every existing ref and source byte.
			source, remote := releaseFixture(t)
			remote = publicationFailureFixture(t, source, remote, scenario)
			before := snapshotSource(t, source)
			refs := fixtureGit(t, remote, "show-ref")
			var output bytes.Buffer
			// Act.
			err := Run(
				context.Background(),
				Config{Dir: source, Mode: "patch", Input: io.NopCloser(strings.NewReader("y\n")), Output: &output},
			)
			// Assert.
			if err == nil {
				t.Fatalf("unexpected success: %s", &output)
			}
			if scenario == "local_collision" || scenario == "push_collision" || scenario == "multiple_destinations" {
				if strings.Contains(output.String(), "Verify ") {
					t.Fatalf("late collision check: %s", &output)
				}
			}
			if after := fixtureGit(t, remote, "show-ref"); after != refs {
				t.Fatalf("remote changed:\nbefore %s\nafter %s", refs, after)
			}
			assertSourceUnchanged(t, source, before)
		})
	}
}

func TestPublicationRechecksAfterPreparation(t *testing.T) {
	// Arrange: another publisher creates a child tag after our first preflight.
	source, remote := releaseFixture(t)
	before := snapshotSource(t, source)
	c, err := isolate(context.Background(), source, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := removeCandidate(c.temp); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	}()
	var output bytes.Buffer
	if err = c.prepare(context.Background(), "patch", &output); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, remote, "tag", "sub/v0.1.1")
	refs := fixtureGit(t, remote, "show-ref")
	// Act.
	err = c.publish(context.Background(), &output)
	// Assert.
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("collision: %v", err)
	}
	if after := fixtureGit(t, remote, "show-ref"); after != refs {
		t.Fatal("remote changed")
	}
	if tags := fixtureGit(t, c.checkout, "tag", "--list", "*v0.1.1"); tags != "" {
		t.Fatalf("tags created before collision check: %s", tags)
	}
	assertSourceUnchanged(t, source, before)
}

func publicationFailureFixture(t *testing.T, source, remote, scenario string) string {
	t.Helper()
	switch scenario {
	case "local_collision":
		fixtureGit(t, source, "tag", "sub/v0.1.1")
	case "push_collision":
		pushRemote := filepath.Join(t.TempDir(), "push.git")
		fixtureGit(t, source, "clone", "--bare", remote, pushRemote)
		fixtureGit(t, pushRemote, "tag", "sub/v0.1.1")
		fixtureGit(t, source, "remote", "set-url", "--push", "origin", pushRemote)
		remote = pushRemote
	case "reject_child":
		hook := filepath.Join(remote, "hooks/update")
		writeFixture(t, hook, "#!/bin/sh\n[ \"$1\" != refs/tags/sub/v0.1.1 ]\n")
		if err := os.Chmod(hook, 0o755); err != nil {
			t.Fatal(err)
		}
	case "no_atomic":
		fixtureGit(t, remote, "config", "receive.advertiseAtomic", "false")
	case "multiple_destinations":
		fixtureGit(t, source, "config", "--add", "remote.origin.pushurl", remote)
		fixtureGit(t, source, "config", "--add", "remote.origin.pushurl", filepath.Join(t.TempDir(), "other.git"))
	}
	return remote
}
