package release

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modmodule "golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

func TestPreparedArtifactsMatchCommittedVCS(t *testing.T) {
	// Arrange.
	source, _ := privateFixture(t)
	writeFixture(t, filepath.Join(source, "LICENSE"), "Fixture license\n")
	fixtureGit(t, source, "add", "LICENSE")
	fixtureGit(t, source, "commit", "-m", "add license")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := isolate(ctx, source, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeCandidate(c.temp); err != nil {
			t.Error(err)
		}
	})
	// Act.
	if err := c.prepare(ctx, "patch", io.Discard); err != nil {
		t.Fatal(err)
	}
	// Assert: compare Go checksum hashes, including inherited root LICENSE.
	for _, m := range c.modules {
		var vcs bytes.Buffer
		subdir := m.dir
		if subdir == "." {
			subdir = ""
		}
		if err := modzip.CreateFromVCS(
			&vcs,
			modmodule.Version{Path: m.path, Version: c.version},
			c.checkout,
			"HEAD",
			subdir,
		); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(t.TempDir(), "vcs.zip")
		if err := os.WriteFile(file, vcs.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		expected, err := dirhash.HashZip(file, dirhash.Hash1)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := dirhash.HashZip(
			filepath.Join(c.temp, "proxy", filepath.FromSlash(m.path), "@v", c.version+".zip"),
			dirhash.Hash1,
		)
		if err != nil {
			t.Fatal(err)
		}
		if actual != expected {
			t.Errorf("%s checksum mismatch: %s != %s", m.path, actual, expected)
		}
	}
}

func TestReleasePrepareOnlyPreservesSource(t *testing.T) {
	// Arrange.
	source, remote := privateFixture(t)
	before := snapshotSource(t, source)
	cfg := fixtureConfig(source)
	cfg.PrepareOnly = true
	cfg.Input = io.NopCloser(strings.NewReader(""))
	// Act.
	err := Run(context.Background(), cfg)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	assertSourceUnchanged(t, source, before)
	if tags := fixtureGit(t, source, "--git-dir", remote, "tag", "-l", "v0.1.1"); tags != "" {
		t.Fatalf("prepare-only published %s", tags)
	}
}
