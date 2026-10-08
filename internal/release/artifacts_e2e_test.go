//go:build e2e

package release_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func artifactProxy(t *testing.T, source, version, proxy string) []string {
	t.Helper()
	dirs := strings.Fields(command(t, source, nil, "make", "--no-print-directory", "-s", "modules"))
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		base := filepath.Join(source, dir)
		data := read(t, filepath.Join(base, "go.mod"))
		manifest, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			t.Fatal(err)
		}
		path := manifest.Module.Mod.Path
		paths = append(paths, path)
		for _, r := range manifest.Replace {
			if strings.HasPrefix(r.Old.Path, "github.com/skosovsky/toolsy") {
				t.Fatalf("development replacement in %s", path)
			}
		}
		for _, r := range manifest.Require {
			if strings.HasPrefix(r.Mod.Path, "github.com/skosovsky/toolsy") && r.Mod.Version != version {
				t.Fatalf("wrong candidate dependency %s", r.Mod)
			}
		}
		escaped, err := module.EscapePath(path)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(proxy, escaped, "@v")
		var archive bytes.Buffer
		if err := modzip.CreateFromDir(&archive, module.Version{Path: path, Version: version}, base); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(target, version+".zip"), archive.Bytes())
		write(t, filepath.Join(target, version+".mod"), data)
		write(
			t,
			filepath.Join(target, version+".info"),
			fmt.Appendf(nil, `{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`, version),
		)
		write(t, filepath.Join(target, "list"), []byte(version+"\n"))
	}
	return paths
}

func consume(t *testing.T, version string, env []string, paths []string) {
	t.Helper()
	dir := t.TempDir()
	command(t, dir, env, "go", "mod", "init", "example.invalid/releaseconsumer")
	for _, path := range paths {
		command(t, dir, env, "go", "mod", "edit", "-require="+path+"@"+version)
	}
	imports := map[string]bool{}
	for _, path := range paths {
		// Compile library, executable and test-only modules without running repository tests.
		command(t, dir, env, "go", "test", "-mod=mod", "-run=^$", path+"/...")
		packages := strings.FieldsSeq(
			command(
				t,
				dir,
				env,
				"go",
				"list",
				"-mod=mod",
				"-f",
				`{{if and (ne .Name "main") (or .GoFiles .CgoFiles)}}{{.ImportPath}}{{end}}`,
				path+"/...",
			),
		)
		for p := range packages {
			if !strings.Contains(p, "/internal/") {
				imports[p] = true
			}
		}
	}
	var code strings.Builder
	code.WriteString("package consumer\nimport (\n")
	for path := range imports {
		fmt.Fprintf(&code, "_ %q\n", path)
	}
	code.WriteString(")\n")
	write(t, filepath.Join(dir, "consumer.go"), []byte(code.String()))
	command(t, dir, env, "go", "mod", "tidy")
	// Tidy removes modules containing only commands/tests; still verify their exact versions.
	for _, path := range paths {
		command(t, dir, env, "go", "mod", "edit", "-require="+path+"@"+version)
	}
	command(t, dir, env, "go", "mod", "download")
	checkResolvedModules(t, dir, env, paths, version)
	command(t, dir, env, "go", "test", "-race", "-count=1", "./...")
	command(t, dir, env, "go", "build", "./...")
}

func checkResolvedModules(t *testing.T, dir string, env []string, paths []string, version string) {
	t.Helper()
	listed := command(t, dir, env, "go", "list", "-m", "-json", "all")
	decoder := json.NewDecoder(strings.NewReader(listed))
	seen := map[string]bool{}
	for decoder.More() {
		var m struct {
			Path    string `json:"Path"`
			Version string `json:"Version"`
			Replace any    `json:"Replace"`
		}
		if err := decoder.Decode(&m); err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if m.Path == path {
				if m.Version != version || m.Replace != nil {
					t.Fatalf("wrong resolved dependency %+v", m)
				}
				seen[path] = true
			}
		}
	}
	if len(seen) != len(paths) {
		t.Fatal("missing published modules")
	}
}

func TestE2ECurrentModuleArtifacts(t *testing.T) {
	// Arrange: construct standard module artifacts from an isolated candidate.
	source := sourceCandidate(t, repoRoot(t))
	version := "v0.0.999"
	proxy := filepath.Join(t.TempDir(), "proxy")
	paths := artifactProxy(t, source, version, proxy)
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(proxy)}).String()
	env := []string{
		"GOENV=off", "GOFLAGS=-modcacherw", "GOPROXY=" + uri + ",https://proxy.golang.org",
		"GONOPROXY=none", "GONOSUMDB=github.com/skosovsky/toolsy,github.com/skosovsky/toolsy/*",
		"GOSUMDB=sum.golang.org", "GOMODCACHE=" + filepath.Join(t.TempDir(), "modcache"),
	}
	// Act / Assert: resolve and compile every discovered module without replacements.
	consume(t, version, env, paths)
}

func sourceCandidate(t *testing.T, root string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "source")
	// Export tracked current files, preserving edits under test but never ignored/untracked payload.
	for name := range strings.SplitSeq(command(t, root, nil, "git", "ls-files", "-z"), "\x00") {
		if name == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(target, name), data)
	}
	for dir := range strings.FieldsSeq(command(t, target, nil, "make", "--no-print-directory", "-s", "modules")) {
		rewriteFixtureManifest(t, filepath.Join(target, dir))
	}

	return target
}

func rewriteFixtureManifest(t *testing.T, base string) {
	t.Helper()
	path := filepath.Join(base, "go.mod")
	m, err := modfile.Parse(path, read(t, path), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Require {
		if strings.HasPrefix(r.Mod.Path, "github.com/skosovsky/toolsy") {
			if err = m.AddRequire(r.Mod.Path, "v0.0.999"); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, r := range append([]*modfile.Replace(nil), m.Replace...) {
		if strings.HasPrefix(r.Old.Path, "github.com/skosovsky/toolsy") {
			if err = m.DropReplace(r.Old.Path, r.Old.Version); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := m.Format()
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, data)
}
