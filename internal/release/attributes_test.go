package release

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseRejectsTransformingAttributes(t *testing.T) {
	for _, attribute := range []string{"export-ignore", "export-subst", "filter=fixture", "working-tree-encoding=UTF-8", "ident", "text", "eol=crlf"} {
		t.Run(attribute, func(t *testing.T) {
			// Arrange.
			source, remote := privateFixture(t)
			writeFixture(t, filepath.Join(source, ".gitattributes"), "value.go "+attribute+"\n")
			fixtureGit(t, source, "add", ".gitattributes")
			fixtureGit(t, source, "commit", "-m", "transforming attributes")
			before := snapshotSource(t, source)
			cfg := fixtureConfig(source)
			cfg.PrepareOnly = true
			// Act.
			err := Run(context.Background(), cfg)
			// Assert.
			if err == nil || !strings.Contains(err.Error(), "transforming Git attribute") {
				t.Fatalf("expected attribute rejection: %v", err)
			}
			assertSourceUnchanged(t, source, before)
			if refs := fixtureGit(t, source, "--git-dir", remote, "tag", "-l", "v0.1.1"); refs != "" {
				t.Fatalf("unexpected publication: %s", refs)
			}
		})
	}
}

func TestReleaseRejectsGlobalTransformingAttributes(t *testing.T) {
	// Arrange.
	source, _ := privateFixture(t)
	attributes := filepath.Join(t.TempDir(), "attributes")
	writeFixture(t, attributes, "value.go export-ignore\n")
	config := filepath.Join(t.TempDir(), "gitconfig")
	writeFixture(t, config, "[core]\n attributesFile = "+attributes+"\n")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	before := snapshotSource(t, source)
	cfg := fixtureConfig(source)
	cfg.PrepareOnly = true
	// Act.
	err := Run(context.Background(), cfg)
	// Assert.
	if err == nil || !strings.Contains(err.Error(), "export-ignore") {
		t.Fatalf("global attribute ignored: %v", err)
	}
	assertSourceUnchanged(t, source, before)
}

func TestReleaseAllowsMetadataAttributes(t *testing.T) {
	// Arrange.
	source, _ := privateFixture(t)
	writeFixture(t, filepath.Join(source, ".gitattributes"), "*.go linguist-language=Go diff=golang -text\n")
	fixtureGit(t, source, "add", ".gitattributes")
	fixtureGit(t, source, "commit", "-m", "metadata attributes")
	cfg := fixtureConfig(source)
	cfg.PrepareOnly = true
	cfg.Output = io.Discard
	// Act.
	err := Run(context.Background(), cfg)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
}
