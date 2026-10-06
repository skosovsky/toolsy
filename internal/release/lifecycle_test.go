package release

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type sourceState struct {
	head, branch, refs string
	index              []byte
	files              map[string][]byte
}

func snapshotSource(t *testing.T, source string) sourceState {
	t.Helper()
	state := sourceState{
		head:   fixtureGit(t, source, "rev-parse", "HEAD"),
		branch: fixtureGit(t, source, "branch", "--show-current"),
		refs:   fixtureGit(t, source, "show-ref"),
		files:  map[string][]byte{},
	}
	var err error
	state.index, err = os.ReadFile(filepath.Join(source, ".git/index"))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		state.files[relative], err = os.ReadFile(path)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func privateFixture(t *testing.T) (string, string) {
	t.Helper()
	source, remote := releaseFixture(t)
	writeFixture(t, filepath.Join(source, ".gitignore"), "private-cache/\n")
	fixtureGit(t, source, "add", ".gitignore")
	fixtureGit(t, source, "commit", "-m", "ignore local cache")
	writeFixture(t, filepath.Join(source, "private-note.txt"), "FAKE_LOCAL_MARKER\n")
	writeFixture(t, filepath.Join(source, "private-cache/secret.txt"), "FAKE_IGNORED_MARKER\n")
	return source, remote
}

func fixtureConfig(source string) Config {
	return Config{Dir: source, Mode: "patch", Input: io.NopCloser(strings.NewReader("y\n")), Output: io.Discard}
}

func assertSourceUnchanged(t *testing.T, source string, before sourceState) {
	t.Helper()
	after := snapshotSource(t, source)
	if !reflect.DeepEqual(before, after) {
		t.Errorf(
			"source state changed: head=%s/%s branch=%s/%s indexEqual=%v filesEqual=%v refsEqual=%v",
			before.head,
			after.head,
			before.branch,
			after.branch,
			bytes.Equal(before.index, after.index),
			reflect.DeepEqual(before.files, after.files),
			before.refs == after.refs,
		)
	}
}

func TestReleaseFailurePreservesSource(t *testing.T) {
	for _, mode := range []string{"verify", "publish", "tracked-dirty", "incomplete-modules"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			source, remote := privateFixture(t)
			cfg := fixtureConfig(source)
			switch mode {
			case "verify":
				writeFixture(t, filepath.Join(source, "value.go"), "package fixture\nvar Invalid = MissingName\n")
				fixtureGit(t, source, "add", "value.go")
				fixtureGit(t, source, "commit", "-m", "broken source")
			case "publish":
				hook := filepath.Join(remote, "hooks/pre-receive")
				writeFixture(t, hook, "#!/bin/sh\nexit 1\n")
				if err := os.Chmod(hook, 0o755); err != nil {
					t.Fatal(err)
				}
			case "tracked-dirty":
				writeFixture(t, filepath.Join(source, "value.go"), "package fixture\nconst Value = 9\n")
				fixtureGit(t, source, "add", "value.go")
			case "incomplete-modules":
				cfg.Modules = "."
			}
			before := snapshotSource(t, source)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Act.
			err := Run(ctx, cfg)
			// Assert.
			if err == nil {
				t.Fatal("expected release failure")
			}
			assertSourceUnchanged(t, source, before)
			if tags := fixtureGit(t, source, "--git-dir", remote, "tag", "-l", "v0.1.1"); tags != "" {
				t.Errorf("failed release published %s", tags)
			}
		})
	}
}

type promptNotice struct {
	once  sync.Once
	ready chan struct{}
}

func (w *promptNotice) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("Publish verified candidate")) {
		w.once.Do(func() { close(w.ready) })
	}
	return len(data), nil
}

func TestReleaseCancelAtConfirmationPreservesSource(t *testing.T) {
	// Arrange: cancellation must close the invocation-owned blocking input.
	source, remote := privateFixture(t)
	before := snapshotSource(t, source)
	input, writer := io.Pipe()
	defer writer.Close()
	notice := &promptNotice{ready: make(chan struct{})}
	cfg := fixtureConfig(source)
	cfg.Input = input
	cfg.Output = notice
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	// Act.
	go func() { done <- Run(ctx, cfg) }()
	select {
	case <-notice.ready:
		cancel()
	case <-ctx.Done():
		t.Fatal("confirmation was not reached")
	}
	err := <-done
	// Assert.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel cause lost: %v", err)
	}
	assertSourceUnchanged(t, source, before)
	if tags := fixtureGit(t, source, "--git-dir", remote, "tag", "-l", "v0.1.1"); tags != "" {
		t.Errorf("canceled release published %s", tags)
	}
}

func TestReleaseExactArtifactGraph(t *testing.T) {
	// Arrange: a three-module chain and a root LICENSE inherited by child zips.
	source, remote := privateFixture(t)
	writeFixture(t, filepath.Join(source, "LICENSE"), "Fixture license\n")
	writeFixture(
		t,
		filepath.Join(source, "leaf/go.mod"),
		"module example.invalid/fixture/leaf\n\ngo 1.27.1\n\nrequire example.invalid/fixture/sub v0.0.0\nreplace example.invalid/fixture/sub => ../sub\nreplace example.invalid/fixture => ../\n",
	)
	writeFixture(
		t,
		filepath.Join(source, "leaf/value.go"),
		"package leaf\nimport sub \"example.invalid/fixture/sub\"\nconst Value = sub.Value\n",
	)
	fixtureGit(t, source, "add", "LICENSE", "leaf")
	fixtureGit(t, source, "commit", "-m", "add dependency chain")
	before := snapshotSource(t, source)
	cfg := fixtureConfig(source)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Act.
	err := Run(ctx, cfg)
	// Assert: Run downloaded and compiled all three exact ZIPs with GOWORK=off.
	if err != nil {
		t.Fatal(err)
	}
	assertSourceUnchanged(t, source, before)
	mod := fixtureGit(t, source, "--git-dir", remote, "show", "leaf/v0.1.1:leaf/go.mod")
	if strings.Contains(mod, "replace") || strings.Contains(mod, "v0.0.0") {
		t.Fatalf("development graph survived: %s", mod)
	}
	if !strings.Contains(mod, "example.invalid/fixture/sub v0.1.1") ||
		!strings.Contains(mod, "example.invalid/fixture v0.1.1") {
		t.Fatalf("unaligned dependency chain: %s", mod)
	}
	sum := fixtureGit(t, source, "--git-dir", remote, "show", "leaf/v0.1.1:leaf/go.sum")
	if !strings.Contains(sum, "example.invalid/fixture/sub v0.1.1 h1:") {
		t.Fatalf("missing exact peer checksum: %s", sum)
	}
}
