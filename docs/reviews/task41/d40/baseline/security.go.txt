package fstool

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
)

func relativePath(path string) (string, error) {
	if filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", toolsy.NewValidationError("access denied: path outside root")
	}
	if slices.Contains(strings.Split(filepath.ToSlash(path), "/"), "..") {
		return "", toolsy.NewValidationError("access denied: path outside root")
	}
	return filepath.Clean(path), nil
}
