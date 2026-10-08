package release_test

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestModulePathsMatchRepositoryDirectories(t *testing.T) {
	// Arrange.
	root := repoRoot(t)
	modules := strings.Fields(command(t, root, nil, "make", "-s", "modules"))
	rootPath := ""
	// Act / Assert: every automatically discovered module has its directory-based path.
	for _, dir := range modules {
		path := filepath.Join(root, dir, "go.mod")
		manifest, err := modfile.Parse(path, read(t, path), nil)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Module == nil {
			t.Fatalf("%s has no module directive", dir)
		}
		if dir == "." {
			rootPath = manifest.Module.Mod.Path
			continue
		}
		if rootPath == "" || manifest.Module.Mod.Path != rootPath+"/"+dir {
			t.Fatalf("%s: unexpected module path %s", dir, manifest.Module.Mod.Path)
		}
	}
}
