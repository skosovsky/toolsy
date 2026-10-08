//go:build e2e

package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseAnswer() string {
	if answer, ok := os.LookupEnv("TOOLSY_RELEASE_FIXTURE_ANSWER"); ok {
		return answer
	}
	return "y\n"
}

func TestE2EReleaseCancelPreservesCaller(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	write(t, filepath.Join(f.repo, "private.txt"), []byte("private\n"))
	before := command(t, f.repo, nil, "git", "status", "--porcelain")
	t.Setenv("TOOLSY_RELEASE_FIXTURE_ANSWER", "n\n")
	// Act.
	out, err := f.invoke(t, "patch", f.source)
	// Assert.
	if err == nil || !strings.Contains(out, "candidate retained") {
		t.Fatalf("%v %s", err, out)
	}
	if got := command(t, f.repo, nil, "git", "status", "--porcelain"); got != before {
		t.Fatal("source files changed")
	}
	if got := command(t, f.repo, nil, "git", "rev-parse", "HEAD"); got != f.source {
		t.Fatal("source HEAD changed")
	}
	if got := command(t, f.repo, nil, "git", "ls-remote", "--refs", f.remote); got != f.initialRefs {
		t.Fatal("cancel published refs")
	}
	candidate := string(read(t, filepath.Join(f.repo, ".git/library-releases/active/candidate")))
	t.Setenv("TOOLSY_RELEASE_FIXTURE_ANSWER", "y\n")
	if out, err = f.invoke(t, "resume"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if got := string(read(t, filepath.Join(f.repo, ".git/library-releases/active/candidate"))); got != candidate {
		t.Fatal("resume replaced candidate")
	}
}

func TestE2EReleaseRejectsWrongModulePath(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	write(t, filepath.Join(f.repo, "packages/test/go.mod"), []byte("module example.invalid/wrong\n\ngo 1.27.1\n"))
	command(t, f.repo, nil, "git", "add", ".")
	command(t, f.repo, nil, "git", "commit", "-m", "invalid module")
	source := command(t, f.repo, nil, "git", "rev-parse", "HEAD")
	// Act.
	out, err := f.invoke(t, "patch", source)
	// Assert.
	if err == nil || !strings.Contains(out, "unexpected module path") {
		t.Fatalf("%v %s", err, out)
	}
	if got := command(t, f.repo, nil, "git", "ls-remote", "--refs", f.remote); got != f.initialRefs {
		t.Fatal("invalid module published")
	}
}

func snapshotCaller(t *testing.T, repo string) string {
	t.Helper()
	var snapshot strings.Builder
	snapshot.WriteString(command(t, repo, nil, "git", "status", "--porcelain"))
	snapshot.WriteString(command(t, repo, nil, "git", "rev-parse", "HEAD"))
	snapshot.WriteString(command(t, repo, nil, "git", "rev-parse", "--symbolic-full-name", "HEAD"))
	snapshot.WriteString(command(t, repo, nil, "git", "show-ref"))
	index := sha256.Sum256(read(t, filepath.Join(repo, ".git/index")))
	snapshot.WriteString(hex.EncodeToString(index[:]))
	err := filepath.WalkDir(repo, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(repo, path)
		if relErr != nil {
			return relErr
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		sum := sha256.Sum256(read(t, path))
		snapshot.WriteString(relative + " " + info.Mode().String() + " " + hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.String()
}
