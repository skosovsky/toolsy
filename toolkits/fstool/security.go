package fstool

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
)

// containsTraversal reports whether path contains ".." as a path segment (traversal).
func containsTraversal(path string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(path), "/"), "..")
}

// sanitizePath resolves baseDir and userPath, resolves symlinks, and ensures the
// result is under baseDir. Use for existing paths (list_dir, read_file).
// Uses filepath.Rel to avoid strings.HasPrefix bypass (e.g. /app/sandbox-bypass).
// baseDir is canonicalized so that on systems where e.g. /var is a symlink to /private/var,
// both base and resolved path are compared in the same canonical form.
// Traversal segments ("..") are rejected before Join so that escape is blocked regardless of platform.
func sanitizePath(baseDir, userPath string) (string, error) {
	if containsTraversal(userPath) {
		return "", &toolsy.ClientError{Reason: "access denied: path outside sandbox", Err: toolsy.ErrValidation}
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", &toolsy.ClientError{Reason: "base dir: " + err.Error(), Err: toolsy.ErrValidation}
	}
	baseCanon, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", &toolsy.ClientError{Reason: "base dir: " + err.Error(), Err: toolsy.ErrValidation}
	}
	joined := filepath.Join(baseCanon, filepath.Clean("/"+userPath))
	// Reject paths that are structurally outside sandbox before resolving symlinks
	if err := pathUnderBase(baseCanon, joined); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &toolsy.ClientError{Reason: "path not found", Err: toolsy.ErrValidation}
		}
		return "", &toolsy.ClientError{Reason: "access denied: path outside sandbox", Err: toolsy.ErrValidation}
	}
	if err := pathUnderBase(baseCanon, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// sanitizePathForWrite validates that the parent of the target path is under baseDir
// (for creating a new file). Returns the joined path to write to.
// The parent directory may not exist yet, so we only check path containment via Rel (no EvalSymlinks on parent).
func sanitizePathForWrite(baseDir, userPath string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", &toolsy.ClientError{Reason: "base dir: " + err.Error(), Err: toolsy.ErrValidation}
	}
	baseCanon, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", &toolsy.ClientError{Reason: "base dir: " + err.Error(), Err: toolsy.ErrValidation}
	}
	if containsTraversal(userPath) {
		return "", &toolsy.ClientError{Reason: "access denied: path outside sandbox", Err: toolsy.ErrValidation}
	}
	cleanPath := filepath.Clean("/" + userPath)
	joined := filepath.Join(baseCanon, cleanPath)
	if err := pathUnderBase(baseCanon, joined); err != nil {
		return "", err
	}
	return joined, nil
}

// pathUnderBase returns nil if resolvedPath is under baseDir; otherwise ClientError.
// Uses filepath.Rel to avoid prefix bypass (e.g. /app/sandbox vs /app/sandbox-bypass).
func pathUnderBase(baseDir, resolvedPath string) error {
	rel, err := filepath.Rel(baseDir, resolvedPath)
	if err != nil {
		return &toolsy.ClientError{Reason: "access denied: path outside sandbox", Err: toolsy.ErrValidation}
	}
	sep := string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return &toolsy.ClientError{Reason: "access denied: path outside sandbox", Err: toolsy.ErrValidation}
	}
	return nil
}
