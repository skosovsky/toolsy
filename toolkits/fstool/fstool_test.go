package fstool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func decodeJSONChunk[T any](t *testing.T, c toolsy.Chunk) T {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out T
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestFSListDir_Success(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(base, "f1.txt"), []byte("x"), 0o600))

	tools, err := AsTools(base)
	require.NoError(t, err)
	listTool := tools[0]

	var result listResult
	require.NoError(
		t,
		listTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"path":""}`)},
			func(c toolsy.Chunk) error {
				result = decodeJSONChunk[listResult](t, c)
				return nil
			},
		),
	)
	require.Len(t, result.Entries, 2)
	names := make([]string, len(result.Entries))
	for i, e := range result.Entries {
		names[i] = e.Name
	}
	require.Contains(t, names, "sub")
	require.Contains(t, names, "f1.txt")
}

func TestFSReadFile_Success(t *testing.T) {
	base := t.TempDir()
	content := "hello world"
	require.NoError(t, os.WriteFile(filepath.Join(base, "f.txt"), []byte(content), 0o600))

	tools, err := AsTools(base)
	require.NoError(t, err)
	readTool := tools[1]

	var result readResult
	require.NoError(
		t,
		readTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"path":"f.txt"}`)},
			func(c toolsy.Chunk) error {
				result = decodeJSONChunk[readResult](t, c)
				return nil
			},
		),
	)
	require.Equal(t, content, result.Content)
}

func TestReadFileLimited_ExceedsOnReadPath(t *testing.T) {
	const byteCap = 32
	content := strings.Repeat("y", byteCap+1)
	_, err := readFileLimited(context.Background(), strings.NewReader(content), byteCap)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(byteCap))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestFSReadFile_ExceedsLimitReturnsValidationError(t *testing.T) {
	base := t.TempDir()
	large := make([]byte, 500)
	for i := range large {
		large[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(base, "big.txt"), large, 0o600))

	const maxBytes = 20
	tools, err := AsTools(base, WithMaxBytes(maxBytes))
	require.NoError(t, err)
	readTool := tools[1]

	err = readTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"big.txt"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(readContentByteCap(maxBytes)))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestFSReadFile_CanceledBeforeStat_ReturnsInternal(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "f.txt"), []byte("hello"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := doReadFile(ctx, base, &options{maxBytes: 1024}, "f.txt")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestFSReadFile_CancelOverStatCap_InterruptWins(t *testing.T) {
	const wireMax = 1000
	contentCap := readContentByteCap(wireMax)
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "big.txt"), []byte(strings.Repeat("x", contentCap+1)), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := doReadFile(ctx, base, &options{maxBytes: wireMax}, "big.txt")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, toolsy.ErrValidation)
}

func TestReadFileLimited_InterruptInChainOverReadLimit(t *testing.T) {
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	_, err := readFileLimited(
		context.Background(),
		io.NopCloser(&instantErrReader{err: composite}),
		1024,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

type instantErrReader struct {
	err error
}

func (r *instantErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestFSReadFile_BetweenWireAndContentCap(t *testing.T) {
	const wireMax = 1000
	contentCap := readContentByteCap(wireMax)
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "edge.txt"), []byte(strings.Repeat("x", contentCap+1)), 0o600))

	tools, err := AsTools(base, WithMaxBytes(wireMax))
	require.NoError(t, err)
	readTool := tools[1]

	err = readTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"edge.txt"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(contentCap))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestFSReadFile_WithinLimit(t *testing.T) {
	base := t.TempDir()
	content := "hello world"
	require.NoError(t, os.WriteFile(filepath.Join(base, "small.txt"), []byte(content), 0o600))

	tools, err := AsTools(base, WithMaxBytes(1024))
	require.NoError(t, err)
	readTool := tools[1]

	var result readResult
	require.NoError(
		t,
		readTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"path":"small.txt"}`)},
			func(c toolsy.Chunk) error {
				result = decodeJSONChunk[readResult](t, c)
				return nil
			},
		),
	)
	require.Equal(t, content, result.Content)
}

func TestFSWriteFile_Success(t *testing.T) {
	base := t.TempDir()
	tools, err := AsTools(base)
	require.NoError(t, err)
	writeTool := tools[2]

	require.NoError(
		t,
		writeTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"path":"a/b/f.txt","content":"written"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
	data, err := os.ReadFile(filepath.Join(base, "a", "b", "f.txt")) // #nosec G304 -- path under test base
	require.NoError(t, err)
	require.Equal(t, "written", string(data))
}

func TestFSWriteFile_SymlinkEscapeBlocked(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "uploads")
	require.NoError(t, os.MkdirAll(inner, 0o750))
	// Symlink inside sandbox pointing outside
	parent := filepath.Dir(base)
	escape := filepath.Join(base, "uploads", "link")
	require.NoError(t, os.Symlink(parent, escape))

	tools, err := AsTools(base)
	require.NoError(t, err)
	writeTool := tools[2]

	err = writeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"uploads/link/evil.txt","content":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestFSWriteFile_ExistingSymlinkFileBlocked(t *testing.T) {
	base := t.TempDir()
	// Create a file then replace with symlink pointing outside sandbox
	reportPath := filepath.Join(base, "report.txt")
	require.NoError(t, os.WriteFile(reportPath, []byte("old"), 0o600))
	require.NoError(t, os.Remove(reportPath))
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.Symlink(outside, reportPath))

	tools, err := AsTools(base)
	require.NoError(t, err)
	writeTool := tools[2]

	err = writeTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"report.txt","content":"x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestFSWriteFile_ReadOnlyBlocked(t *testing.T) {
	tools, err := AsTools(t.TempDir(), WithReadOnly(true))
	require.NoError(t, err)
	require.Len(t, tools, 2)
}

func TestFSListDir_PathTraversal(t *testing.T) {
	base := t.TempDir()
	tools, err := AsTools(base)
	require.NoError(t, err)
	listTool := tools[0]

	err = listTool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"../../etc"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestAsTools_InvalidBaseDir(t *testing.T) {
	_, err := AsTools("/nonexistent-dir-xyz")
	require.Error(t, err)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(t.TempDir())
	require.NoError(t, err)
	require.Len(t, tools, 3)
	require.Equal(t, "fs_list_dir", tools[0].Manifest().Name)
	require.Equal(t, "fs_read_file", tools[1].Manifest().Name)
	require.Equal(t, "fs_write_file", tools[2].Manifest().Name)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().ReadOnly)
	require.True(t, tools[2].Manifest().Dangerous)
	require.True(t, tools[2].Manifest().RequiresConfirmation)
}

func TestAsTools_ToolCountReadOnly(t *testing.T) {
	tools, err := AsTools(t.TempDir(), WithReadOnly(true))
	require.NoError(t, err)
	require.Len(t, tools, 2)
}

func TestListDir_CancelBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := doListDir(ctx, t.TempDir(), &options{}, "")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWriteFile_CancelBeforeWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := doWriteFile(ctx, t.TempDir(), "out.txt", "data")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
