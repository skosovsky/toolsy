package fstool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestFSListDir_Success(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(base, "f1.txt"), []byte("x"), 0o600))

	tools, err := AsTools(base)
	require.NoError(t, err)
	listTool := tools[0]

	var result listResult
	require.NoError(t, listTool.Execute(context.Background(), []byte(`{"path":""}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(listResult); ok {
				result = r
			}
		}
		return nil
	}))
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
	require.NoError(t, readTool.Execute(context.Background(), []byte(`{"path":"f.txt"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Equal(t, content, result.Content)
}

func TestFSReadFile_Truncation(t *testing.T) {
	base := t.TempDir()
	large := make([]byte, 500)
	for i := range large {
		large[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(base, "big.txt"), large, 0o600))

	tools, err := AsTools(base, WithMaxBytes(20))
	require.NoError(t, err)
	readTool := tools[1]

	var result readResult
	require.NoError(t, readTool.Execute(context.Background(), []byte(`{"path":"big.txt"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(readResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Contains(t, result.Content, "[Truncated]")
	require.LessOrEqual(t, len(result.Content), 20+len(truncationSuffix)+5)
}

func TestFSWriteFile_Success(t *testing.T) {
	base := t.TempDir()
	tools, err := AsTools(base)
	require.NoError(t, err)
	writeTool := tools[2]

	require.NoError(t, writeTool.Execute(context.Background(), []byte(`{"path":"a/b/f.txt","content":"written"}`), func(toolsy.Chunk) error { return nil }))
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

	err = writeTool.Execute(context.Background(), []byte(`{"path":"uploads/link/evil.txt","content":"x"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "outside sandbox")
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

	err = writeTool.Execute(context.Background(), []byte(`{"path":"report.txt","content":"x"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	// Framework may wrap error; ensure write was rejected (sandbox escape blocked)
	require.True(t, strings.Contains(err.Error(), "outside sandbox") || strings.Contains(err.Error(), "internal system error"))
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

	err = listTool.Execute(context.Background(), []byte(`{"path":"../../etc"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestAsTools_InvalidBaseDir(t *testing.T) {
	_, err := AsTools("/nonexistent-dir-xyz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "toolkit/fstool")
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(t.TempDir())
	require.NoError(t, err)
	require.Len(t, tools, 3)
	require.Equal(t, "fs_list_dir", tools[0].Name())
	require.Equal(t, "fs_read_file", tools[1].Name())
	require.Equal(t, "fs_write_file", tools[2].Name())
}

func TestAsTools_ToolCountReadOnly(t *testing.T) {
	tools, err := AsTools(t.TempDir(), WithReadOnly(true))
	require.NoError(t, err)
	require.Len(t, tools, 2)
}
