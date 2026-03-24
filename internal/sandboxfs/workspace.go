package sandboxfs

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var errInvalidPath = errors.New("invalid workspace path")
var errFileConflict = errors.New("workspace file conflict")

// NormalizeRelativePath validates a sandbox workspace path and returns a clean,
// slash-delimited relative path suitable for mounting into an isolated workspace.
func NormalizeRelativePath(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("%w: path is empty", errInvalidPath)
	}
	if strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("%w: path contains NUL byte", errInvalidPath)
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if filepath.IsAbs(normalized) || path.IsAbs(normalized) || strings.HasPrefix(normalized, "//") ||
		hasWindowsDrivePrefix(normalized) {
		return "", fmt.Errorf("%w: absolute paths are not allowed", errInvalidPath)
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: path must stay inside workspace", errInvalidPath)
	}
	return clean, nil
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	return (name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')
}

// Resolve returns the host filesystem path for a workspace-relative path.
func Resolve(root, name string) (string, error) {
	clean, err := NormalizeRelativePath(name)
	if err != nil {
		return "", err
	}

	full := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errInvalidPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes workspace", errInvalidPath)
	}
	return full, nil
}

// WriteFile writes a workspace-relative file under root, creating parent
// directories as needed.
func WriteFile(root, name string, data []byte) error {
	full, err := Resolve(root, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o600)
}

// CanonicalizeFiles normalizes workspace-relative paths, rejects collisions after
// normalization, and optionally reserves internal adapter entrypoint paths.
func CanonicalizeFiles(files map[string][]byte, reserved ...string) (map[string][]byte, error) {
	reservedPaths := make(map[string]struct{}, len(reserved))
	for _, name := range reserved {
		clean, err := NormalizeRelativePath(name)
		if err != nil {
			return nil, err
		}
		reservedPaths[clean] = struct{}{}
	}

	if len(files) == 0 {
		return make(map[string][]byte), nil
	}

	canonical := make(map[string][]byte, len(files))
	for name, data := range files {
		clean, err := NormalizeRelativePath(name)
		if err != nil {
			return nil, err
		}
		if _, ok := reservedPaths[clean]; ok {
			return nil, fmt.Errorf("%w: reserved path %q", errFileConflict, clean)
		}
		if _, ok := canonical[clean]; ok {
			return nil, fmt.Errorf("%w: duplicate path %q after normalization", errFileConflict, clean)
		}
		canonical[clean] = append([]byte(nil), data...)
	}

	return canonical, nil
}

// WriteWorkspace materializes the provided in-memory file map inside root.
func WriteWorkspace(root string, files map[string][]byte) error {
	canonical, err := CanonicalizeFiles(files)
	if err != nil {
		return err
	}
	for name, data := range canonical {
		if err := WriteFile(root, name, data); err != nil {
			return err
		}
	}
	return nil
}
