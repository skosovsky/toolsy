package fstool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestSanitizePath_ValidRelative(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o750))

	resolved, err := sanitizePath(base, "a/b")
	require.NoError(t, err)
	canonSub, _ := filepath.EvalSymlinks(sub)
	require.Equal(t, canonSub, resolved)
}

func TestSanitizePath_EmptyPath(t *testing.T) {
	base := t.TempDir()
	resolved, err := sanitizePath(base, "")
	require.NoError(t, err)
	abs, _ := filepath.Abs(base)
	canon, _ := filepath.EvalSymlinks(abs)
	require.Equal(t, canon, resolved)
}

func TestSanitizePath_PathNotFound(t *testing.T) {
	base := t.TempDir()
	// Path inside sandbox that does not exist -> "path not found", not "outside sandbox"
	_, err := sanitizePath(base, "missing/file.txt")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "path not found")
}

func TestSanitizePath_PathTraversalBlocked(t *testing.T) {
	base := t.TempDir()

	_, err := sanitizePath(base, "../../etc/passwd")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "outside sandbox")
}

func TestSanitizePath_TraversalSegmentBlockedBeforeJoin(t *testing.T) {
	base := t.TempDir()
	// Any ".." segment is rejected before Join (consistent message)
	_, err := sanitizePath(base, "a/../b")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "outside sandbox")
}

func TestSanitizePath_AbsolutePathInterpretedAsRelative(t *testing.T) {
	base := t.TempDir()
	// On Unix, Join(base, "/etc/passwd") yields "/etc/passwd"; EvalSymlinks may succeed.
	// Rel(base, "/etc/passwd") gives ".." or "../.." + "/etc/passwd" -> blocked.
	_, err := sanitizePath(base, "/etc/passwd")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestSanitizePath_SymlinkEscapeBlocked(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o750))
	escape := filepath.Join(base, "escape")
	// Symlink escape -> points to parent of base (outside sandbox)
	parent := filepath.Dir(base)
	require.NoError(t, os.Symlink(parent, escape))

	_, err := sanitizePath(base, "escape")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestSanitizePathForWrite_ValidParent(t *testing.T) {
	base := t.TempDir()
	baseCanon, _ := filepath.EvalSymlinks(base)
	target, err := sanitizePathForWrite(base, "a/b/file.txt")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(baseCanon, "a", "b", "file.txt"), target)
}

func TestSanitizePathForWrite_TraversalBlocked(t *testing.T) {
	base := t.TempDir()
	_, err := sanitizePathForWrite(base, "../../etc/secret")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "outside sandbox")
}

func TestSanitizePathForWrite_TraversalSegmentBlocked(t *testing.T) {
	base := t.TempDir()
	_, err := sanitizePathForWrite(base, "sub/../../file.txt")
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "outside sandbox")
}
