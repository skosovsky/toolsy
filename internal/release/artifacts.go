package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	modmodule "golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func (c *candidate) moduleFiles(m *module, files []string) []string {
	seen := make(map[string]bool)
	prefix := ""
	if m.dir != "." {
		prefix = m.dir + "/"
	}
	for _, file := range append(append([]string(nil), files...), prefix+"go.mod", prefix+"go.sum") {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		nested := false
		for _, peer := range c.modules {
			if peer.dir != m.dir && strings.HasPrefix(file, peer.dir+"/") && strings.HasPrefix(peer.dir+"/", prefix) {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		relative := strings.TrimPrefix(file, prefix)
		if relative != "" {
			seen[relative] = true
		}
	}
	result := make([]string, 0, len(seen))
	for file := range seen {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

type artifactFile struct{ name, source string }

func (f artifactFile) Path() string                 { return f.name }
func (f artifactFile) Lstat() (os.FileInfo, error)  { return os.Lstat(f.source) }
func (f artifactFile) Open() (io.ReadCloser, error) { return os.Open(f.source) }

func (c *candidate) artifactFiles(m *module, tracked []string) ([]modzip.File, error) {
	var files []modzip.File
	haveLicense := false
	for _, name := range c.moduleFiles(m, tracked) {
		source := filepath.Join(c.checkout, m.dir, filepath.FromSlash(name))
		if _, err := os.Lstat(source); err != nil {
			if name == "go.sum" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		files = append(files, artifactFile{name: name, source: source})
		haveLicense = haveLicense || name == "LICENSE"
	}
	// Match Go's VCS artifact policy: child modules inherit the root LICENSE.
	if !haveLicense && m.dir != "." {
		source := filepath.Join(c.checkout, "LICENSE")
		if _, err := os.Lstat(source); err == nil {
			files = append(files, artifactFile{name: "LICENSE", source: source})
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return files, nil
}

func (c *candidate) writeArtifact(m *module, proxy string, tracked []string) (string, error) {
	escaped, err := modmodule.EscapePath(m.path)
	if err != nil {
		return "", err
	}
	location := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
	if err = os.MkdirAll(location, 0o750); err != nil {
		return "", err
	}
	files, err := c.artifactFiles(m, tracked)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	if err = modzip.Create(&buffer, modmodule.Version{Path: m.path, Version: c.version}, files); err != nil {
		return "", err
	}
	info, err := json.Marshal(struct {
		Version string    `json:"Version"`
		Time    time.Time `json:"Time"`
	}{c.version, time.Unix(0, 0).UTC()})
	if err != nil {
		return "", err
	}
	mod, err := os.ReadFile(filepath.Join(c.checkout, m.dir, "go.mod"))
	if err != nil {
		return "", err
	}
	contents := map[string][]byte{
		c.version + ".zip":  buffer.Bytes(),
		c.version + ".mod":  mod,
		c.version + ".info": info,
		"list":              []byte(c.version + "\n"),
	}
	for name, data := range contents {
		if err = os.WriteFile(filepath.Join(location, name), data, 0o600); err != nil {
			return "", err
		}
	}
	return location, nil
}
