package toolsygen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinalizeFailuresRestoreEarlierFiles(t *testing.T) {
	for _, fault := range []string{"reserve", "backup", "install", "cancel_after_first", "cancel_after_last", "cancel_after_backup"} {
		t.Run(fault, func(t *testing.T) {
			// Arrange.
			dir := t.TempDir()
			a := filepath.Join(dir, "a.go")
			b := filepath.Join(dir, "b.go")
			writeFile(t, a, "old-a")
			writeFile(t, b, "old-b")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cause := errors.New("injected " + fault)
			facade := finalizeFaultFacade(fault, a, b, cause, cancel)
			g := &generator{fs: facade}
			// Act.
			err := g.commitFilesWithRollback(
				ctx,
				[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
				0o600,
			)
			// Assert.
			if strings.HasPrefix(fault, "cancel") {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, cause)
			}
			requireFileContent(t, a, "old-a")
			requireFileContent(t, b, "old-b")
			requireNoArtifacts(t, dir)
		})
	}
}
func TestFailedRollbackPreservesRecoveryCopies(t *testing.T) {
	for _, fault := range []string{"restore_a", "restore_b", "remove_a"} {
		t.Run(fault, func(t *testing.T) {
			// Arrange: second install fails after both old files have backups.
			dir := t.TempDir()
			a := filepath.Join(dir, "a.go")
			b := filepath.Join(dir, "b.go")
			writeFile(t, a, "old-a")
			writeFile(t, b, "old-b")
			primary, secondary := errors.New("install failed"), errors.New("recovery failed")
			facade := rollbackFaultFacade(fault, a, b, primary, secondary)
			g := &generator{fs: facade}
			// Act.
			err := g.commitFilesWithRollback(
				t.Context(),
				[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
				0o600,
			)
			// Assert: cleanup cannot delete the sole old-content recovery copy.
			require.ErrorIs(t, err, primary)
			require.ErrorIs(t, err, secondary)
			target, old := a, "old-a"
			if fault == "restore_b" {
				target, old = b, "old-b"
			}
			backups, globErr := filepath.Glob(target + ".bak-*")
			require.NoError(t, globErr)
			require.Len(t, backups, 1)
			requireFileContent(t, backups[0], old)
			require.Contains(t, err.Error(), backups[0])
			require.Contains(t, err.Error(), target)
			if fault == "remove_a" {
				requireFileContent(t, a, "new-a")
			} else {
				_, statErr := os.Stat(target)
				require.ErrorIs(t, statErr, os.ErrNotExist)
			}
			temps, globErr := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
			require.NoError(t, globErr)
			require.Empty(t, temps)
		})
	}
}
func TestStagingFailureCleansEarlierOwnedTemps(t *testing.T) {
	// Arrange.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, a, "old-a")
	writeFile(t, b, "old-b")
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	create := facade.createTemp
	cause := errors.New("second staging failed")
	facade.createTemp = func(dir, pattern string) (*os.File, error) {
		if strings.HasPrefix(pattern, "b.go.tmp-") {
			return nil, cause
		}
		return create(dir, pattern)
	}
	g := &generator{fs: facade}
	// Act.
	err := g.commitFilesWithRollback(
		t.Context(),
		[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
		0o600,
	)
	// Assert.
	require.ErrorIs(t, err, cause)
	requireFileContent(t, a, "old-a")
	requireFileContent(t, b, "old-b")
	requireNoArtifacts(t, dir)
}
func TestPostCommitCleanupReportsInstalledState(t *testing.T) {
	// Arrange.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, a, "old-a")
	writeFile(t, b, "old-b")
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	remove := facade.remove
	cause := errors.New("backup disposal failed")
	facade.remove = func(path string) error {
		if strings.Contains(path, ".bak-") {
			data, err := os.ReadFile(path)
			if err == nil && len(data) > 0 {
				return cause
			}
		}
		return remove(path)
	}
	g := &generator{fs: facade}
	// Act.
	err := g.commitFilesWithRollback(
		t.Context(),
		[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
		0o600,
	)
	// Assert: finalization succeeded; two cleanup causes retain old copies and new outputs.
	require.ErrorIs(t, err, cause)
	requireFileContent(t, a, "new-a")
	requireFileContent(t, b, "new-b")
	var diagnostic *FileRecoveryError
	require.ErrorAs(t, err, &diagnostic)
	require.True(t, diagnostic.CommitComplete)
	backups, globErr := filepath.Glob(filepath.Join(dir, "*.bak-*"))
	require.NoError(t, globErr)
	require.Len(t, backups, 2)
	require.Contains(t, err.Error(), a)
	require.Contains(t, err.Error(), b)
}
func TestNewTargetsRollbackToAbsence(t *testing.T) {
	// Arrange: first output did not exist; second old-file backup fails.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, b, "old-b")
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	rename := facade.rename
	cause := errors.New("backup failed")
	facade.rename = func(oldPath, newPath string) error {
		if oldPath == b {
			return cause
		}
		return rename(oldPath, newPath)
	}
	g := &generator{fs: facade}
	// Act.
	err := g.commitFilesWithRollback(
		t.Context(),
		[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
		0o600,
	)
	// Assert.
	require.ErrorIs(t, err, cause)
	_, statErr := os.Stat(a)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	requireFileContent(t, b, "old-b")
	requireNoArtifacts(t, dir)
}
func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(data))
}
func requireNoArtifacts(t *testing.T, dir string) {
	t.Helper()
	for _, pattern := range []string{"*.tmp-*", "*.bak-*"} {
		paths, err := filepath.Glob(filepath.Join(dir, pattern))
		require.NoError(t, err)
		require.Empty(t, paths)
	}
}

func finalizeFaultFacade(fault, a, b string, cause error, cancel context.CancelFunc) osFacade {
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	create, rename := facade.createTemp, facade.rename
	facade.createTemp = func(dir, pattern string) (*os.File, error) {
		if fault == "reserve" && strings.HasPrefix(pattern, "b.go.bak-") {
			return nil, cause
		}
		return create(dir, pattern)
	}
	facade.rename = func(oldPath, newPath string) error {
		if fault == "backup" && oldPath == b && strings.Contains(newPath, ".bak-") {
			return cause
		}
		if fault == "install" && strings.Contains(oldPath, "b.go.tmp-") && newPath == b {
			return cause
		}
		err := rename(oldPath, newPath)
		if err == nil && shouldCancelFinalize(fault, oldPath, newPath, a, b) {
			cancel()
		}
		return err
	}
	return facade
}

func rollbackFaultFacade(fault, a, b string, primary, secondary error) osFacade {
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	rename, remove := facade.rename, facade.remove
	facade.rename = func(oldPath, newPath string) error {
		if strings.Contains(oldPath, "b.go.tmp-") && newPath == b {
			return primary
		}
		if strings.Contains(oldPath, ".bak-") &&
			((fault == "restore_a" && newPath == a) || (fault == "restore_b" && newPath == b)) {
			return secondary
		}
		return rename(oldPath, newPath)
	}
	facade.remove = func(path string) error {
		if fault == "remove_a" && path == a {
			return secondary
		}
		return remove(path)
	}
	return facade
}

func shouldCancelFinalize(fault, oldPath, newPath, a, b string) bool {
	switch fault {
	case "cancel_after_first":
		return strings.Contains(oldPath, ".tmp-") && newPath == a
	case "cancel_after_last":
		return strings.Contains(oldPath, ".tmp-") && newPath == b
	case "cancel_after_backup":
		return oldPath == b && strings.Contains(newPath, ".bak-")
	default:
		return false
	}
}

func TestStagingCleanupFailureReportsOwnedPath(t *testing.T) {
	// Arrange: second staging fails; removing the first owned temporary file also fails.
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, a, "old-a")
	writeFile(t, b, "old-b")
	facade := newDefaultOSFacade(defaultMaxGeneratorFileBytes)
	create, remove := facade.createTemp, facade.remove
	primary, secondary := errors.New("staging failed"), errors.New("cleanup failed")
	facade.createTemp = func(dir, pattern string) (*os.File, error) {
		if strings.HasPrefix(pattern, "b.go.tmp-") {
			return nil, primary
		}
		return create(dir, pattern)
	}
	facade.remove = func(path string) error {
		if strings.Contains(path, "a.go.tmp-") {
			return secondary
		}
		return remove(path)
	}
	g := &generator{fs: facade}
	// Act.
	err := g.commitFilesWithRollback(
		t.Context(),
		[]generatedFile{{Path: a, Content: []byte("new-a")}, {Path: b, Content: []byte("new-b")}},
		0o600,
	)
	// Assert: only owned new-content artifact remains, with cause and path disclosed.
	require.ErrorIs(t, err, primary)
	require.ErrorIs(t, err, secondary)
	requireFileContent(t, a, "old-a")
	requireFileContent(t, b, "old-b")
	paths, globErr := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	require.NoError(t, globErr)
	require.Len(t, paths, 1)
	require.Contains(t, err.Error(), paths[0])
	requireFileContent(t, paths[0], "new-a")
}
