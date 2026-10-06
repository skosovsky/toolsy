//go:build unix

package document

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalRootRejectsFIFOWithoutPeer(t *testing.T) {
	// Arrange.
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe.csv"), 0o600))
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	tool, err := AsTool(WithLocalRoot(root))
	require.NoError(t, err)
	// Act.
	wire, err := executeReference(t, tool, "pipe.csv")
	// Assert.
	require.Error(t, err)
	require.Contains(t, err.Error(), "regular file")
	require.Empty(t, wire)
}
