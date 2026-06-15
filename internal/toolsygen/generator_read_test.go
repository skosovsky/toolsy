package toolsygen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestReadManifestFile_ExceedsLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yaml")
	const maxFileBytes = 64
	require.NoError(t, os.WriteFile(path, make([]byte, maxFileBytes+1), 0o600))

	g := newGenerator(Config{MaxFileBytes: maxFileBytes})
	_, err := g.readManifestFile(context.Background(), path)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadFileLimitedFromDisk_StatPreCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.bin")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(int64(defaultMaxGeneratorFileBytes)+1))
	require.NoError(t, f.Close())

	_, err = readFileLimitedFromDisk(context.Background(), path, defaultMaxGeneratorFileBytes)
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestGenerate_RespectsMaxFileBytesConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "tool.yaml")
	manifestYAML := []byte(
		"name: x\ndescription: d\nparameters:\n  type: object\n  properties: {}\n",
	)
	require.NoError(t, os.WriteFile(manifestPath, manifestYAML, 0o600))

	const maxFileBytes = 128
	_, err := Generate(context.Background(), Config{Inputs: []string{dir}, MaxFileBytes: maxFileBytes})
	require.NoError(t, err)

	bigPath := filepath.Join(dir, "big.yaml")
	require.NoError(t, os.WriteFile(bigPath, make([]byte, maxFileBytes+1), 0o600))
	_, err = Generate(context.Background(), Config{Inputs: []string{dir}, MaxFileBytes: maxFileBytes})
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadFileLimitedFromDisk_CancelBeforeRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readFileLimitedFromDisk(ctx, path, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReadFileLimitedFromDisk_CancelOverStatSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	require.NoError(t, os.WriteFile(path, make([]byte, 200), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readFileLimitedFromDisk(ctx, path, 64)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestReadFileLimited_CanceledAfterSuccessfulReadOverCap(t *testing.T) {
	t.Parallel()
	const maxFileBytes = 64
	g := newGenerator(Config{MaxFileBytes: maxFileBytes})
	g.fs.readFile = func(_ context.Context, _ string) ([]byte, error) {
		return make([]byte, maxFileBytes+1), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.readFileLimited(ctx, "big.yaml")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFinishGeneratorRead_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	_, err := finishGeneratorRead(context.Background(), "tool.yaml", 1024, nil, composite)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
