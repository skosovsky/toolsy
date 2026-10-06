package sandboxfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveWorkspace_PagesNestedFilesAndPreservesSymlinkDestination(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	nested := filepath.Join(workspace, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(workspace, "link")))
	for i := range 200 {
		require.NoError(t, os.WriteFile(filepath.Join(nested, string(rune('a'+i))), []byte("data"), 0o600))
	}

	err := RemoveWorkspace(context.Background(), workspace)

	require.NoError(t, err)
	_, statErr := os.Stat(workspace)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	content, readErr := os.ReadFile(outside)
	require.NoError(t, readErr)
	require.Equal(t, "keep", string(content))
}

func TestRemoveWorkspace_CanceledBeforeRemovalAndAbsentWorkspace(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	file := filepath.Join(workspace, "keep")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RemoveWorkspace(ctx, workspace)

	require.ErrorIs(t, err, context.Canceled)
	_, statErr := os.Stat(file)
	require.NoError(t, statErr)
	require.NoError(t, RemoveWorkspace(context.Background(), filepath.Join(workspace, "absent")))
}

func TestRemoveWorkspace_RejectsReplacedRootSymlink(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))
	keep := filepath.Join(outside, "keep")
	require.NoError(t, os.WriteFile(keep, []byte("keep"), 0o600))
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Symlink(outside, workspace))

	err := RemoveWorkspace(context.Background(), workspace)

	require.Error(t, err)
	content, readErr := os.ReadFile(keep)
	require.NoError(t, readErr)
	require.Equal(t, "keep", string(content))
}
