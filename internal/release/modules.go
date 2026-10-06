package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type moduleVersion struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
}
type moduleReplace struct {
	Old moduleVersion `json:"Old"`
	New moduleVersion `json:"New"`
}
type moduleManifest struct {
	Module  moduleVersion   `json:"Module"`
	Require []moduleVersion `json:"Require"`
	Replace []moduleReplace `json:"Replace"`
}
type module struct {
	dir, path string
	manifest  moduleManifest
}

func (c *candidate) loadModule(ctx context.Context, dir string) (*module, error) {
	stat, err := os.Lstat(filepath.Join(c.checkout, dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, errors.New("module manifests must be regular committed files")
	}
	sum, err := os.Lstat(filepath.Join(c.checkout, dir, "go.sum"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && !sum.Mode().IsRegular() {
		return nil, errors.New("module checksums must be regular files")
	}
	raw, err := c.commands.run(ctx, filepath.Join(c.checkout, dir), "go", "mod", "edit", "-json")
	if err != nil {
		return nil, err
	}
	var manifest moduleManifest
	if err = json.Unmarshal([]byte(raw), &manifest); err != nil {
		return nil, err
	}
	if manifest.Module.Path == "" {
		return nil, errors.New("module path is required")
	}
	return &module{dir: dir, path: manifest.Module.Path, manifest: manifest}, nil
}

func ownedModule(root, path string) bool { return path == root || strings.HasPrefix(path, root+"/") }

func orderModules(modules []*module, root string) ([]*module, error) {
	byPath := make(map[string]*module, len(modules))
	for _, m := range modules {
		expected := root
		if m.dir != "." {
			expected += "/" + m.dir
		}
		if m.path != expected {
			return nil, fmt.Errorf("module %s has path %s, expected %s", m.dir, m.path, expected)
		}
		byPath[m.path] = m
	}
	order := moduleOrder{root: root, byPath: byPath, state: make(map[string]int, len(modules)), ordered: nil}

	sorted := append([]*module(nil), modules...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	for _, m := range sorted {
		if err := order.visit(m); err != nil {
			return nil, err
		}
	}
	return order.ordered, nil
}

func (c *candidate) rewrite(ctx context.Context, m *module) error {
	args := []string{"mod", "edit"}
	for _, requirement := range m.manifest.Require {
		if ownedModule(c.root, requirement.Path) {
			args = append(args, "-require="+requirement.Path+"@"+c.version)
		}
	}
	for _, replacement := range m.manifest.Replace {
		if ownedModule(c.root, replacement.Old.Path) {
			value := replacement.Old.Path
			if replacement.Old.Version != "" {
				value += "@" + replacement.Old.Version
			}
			args = append(args, "-dropreplace="+value)
		} else if replacement.New.Version == "" {
			return fmt.Errorf("local external replace in %s cannot be released", m.path)
		}
	}
	args = append(args, "-fmt")
	_, err := c.commands.run(ctx, filepath.Join(c.checkout, m.dir), "go", args...)
	return err
}

func (c *candidate) verifyModule(ctx context.Context, m *module, proxy string, files []string) error {
	dir := filepath.Join(c.checkout, m.dir)
	if _, err := c.commands.run(ctx, dir, "go", "mod", "tidy"); err != nil {
		return err
	}
	updated, err := c.loadModule(ctx, m.dir)
	if err != nil {
		return err
	}
	for _, requirement := range updated.manifest.Require {
		if ownedModule(c.root, requirement.Path) && requirement.Version != c.version {
			return fmt.Errorf("unaligned release requirement %s@%s", requirement.Path, requirement.Version)
		}
	}
	for _, replacement := range updated.manifest.Replace {
		if ownedModule(c.root, replacement.Old.Path) {
			return errors.New("internal development replace survived preparation")
		}
	}
	_, err = c.writeArtifact(m, proxy, files)
	if err != nil {
		return err
	}
	raw, err := c.commands.run(ctx, c.temp, "go", "mod", "download", "-json", m.path+"@"+c.version)
	if err != nil {
		return err
	}
	var downloaded struct {
		Dir   string `json:"Dir"`
		Error string `json:"Error"`
	}
	if err = json.Unmarshal([]byte(raw), &downloaded); err != nil {
		return err
	}
	if downloaded.Dir == "" || downloaded.Error != "" {
		return fmt.Errorf("artifact download failed for %s: %s", m.path, downloaded.Error)
	}
	if _, err = c.commands.run(ctx, downloaded.Dir, "go", "test", "-mod=readonly", "-run=^$", "./..."); err != nil {
		return err
	}
	return nil
}

type moduleOrder struct {
	root    string
	byPath  map[string]*module
	state   map[string]int
	ordered []*module
}

func (o *moduleOrder) visit(m *module) error {
	if o.state[m.path] == 2 {
		return nil
	}
	if o.state[m.path] == 1 {
		return fmt.Errorf("cyclic internal module dependency at %s", m.path)
	}
	o.state[m.path] = 1
	for _, dependency := range m.manifest.Require {
		if !ownedModule(o.root, dependency.Path) {
			continue
		}
		peer, ok := o.byPath[dependency.Path]
		if !ok {
			return fmt.Errorf("internal dependency %s is missing from tracked module inventory", dependency.Path)
		}
		if err := o.visit(peer); err != nil {
			return err
		}
	}
	o.state[m.path] = 2
	o.ordered = append(o.ordered, m)
	return nil
}
