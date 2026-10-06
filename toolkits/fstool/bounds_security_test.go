package fstool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestWriteOutsideSymlinkCreatesNothing(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "link")))
	_, err := doWriteFile(context.Background(), base, "link/new/file", "approved")
	require.Error(t, err)
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestConcurrentSymlinkReplacementCannotEscape(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(base, "inside"), 0o750))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink("inside", link))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(link)
			_ = os.Symlink(outside, link)
			_ = os.Remove(link)
			_ = os.Symlink("inside", link)
		}
	})
	for range 200 {
		_, _ = doWriteFile(context.Background(), base, "link/new/file", "approved")
	}
	close(stop)
	wg.Wait()
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestWireBudgetIncludesEscapingAndSourceReference(t *testing.T) {
	for _, content := range []string{strings.Repeat("<", 40), strings.Repeat("語", 40), strings.Repeat("\\", 80)} {
		t.Run(content[:1], func(t *testing.T) {
			base := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(base, "file"), []byte(content), 0o600))
			tools, err := AsTools(base, WithMaxBytes(100))
			require.NoError(t, err)
			yields := 0
			err = tools[1].Execute(
				context.Background(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{"path":"file"}`)},
				func(c toolsy.Chunk) error {
					yields++
					require.LessOrEqual(t, len(c.Data), 100)
					require.True(t, json.Valid(c.Data))
					return nil
				},
			)
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Zero(t, yields)
		})
	}
}

func TestReadRangeAndDirectoryContinuation(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "file"), []byte("abcdef"), 0o600))
	var o options
	applyDefaults(&o)
	o.maxSourceBytes = 3
	_, err := doReadFile(context.Background(), base, &o, "file")
	require.ErrorIs(t, err, toolsy.ErrValidation)
	first, err := readFile(context.Background(), base, &o, readArgs{Path: "file", Length: 3})
	require.NoError(t, err)
	require.Equal(t, "abc", first.Content)
	require.True(t, first.HasMore)
	second, err := readFile(context.Background(), base, &o, readArgs{Path: "file", Offset: first.NextOffset, Length: 3})
	require.NoError(t, err)
	require.Equal(t, "def", second.Content)
	require.False(t, second.HasMore)
	require.NoError(t, os.WriteFile(filepath.Join(base, "other"), []byte("x"), 0o600))
	page, err := listDirectory(context.Background(), base, &o, listArgs{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	require.True(t, page.HasMore)
	next, err := listDirectory(context.Background(), base, &o, listArgs{Offset: page.NextOffset, Limit: 1})
	require.NoError(t, err)
	require.Len(t, next.Entries, 1)
	require.NotEqual(t, page.Entries[0].Name, next.Entries[0].Name)
	require.False(t, next.HasMore)
}

func TestInvalidBoundsAndWriteNoSilentMutation(t *testing.T) {
	base := t.TempDir()
	_, err := AsTools(base, WithMaxSourceBytes(-1))
	require.Error(t, err)
	tools, err := AsTools(base, WithMaxSourceBytes(3))
	require.NoError(t, err)
	err = tools[2].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"new/file","content":"long"}`)},
		func(toolsy.Chunk) error { t.Fatal("unexpected output"); return nil },
	)
	require.ErrorIs(t, err, toolsy.ErrValidation)
	_, err = os.Stat(filepath.Join(base, "new"))
	require.True(t, os.IsNotExist(err))
}

func TestDirectoryLimitsFailClosed(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(base, "<escaped>"), []byte("x"), 0o600))
	var o options
	applyDefaults(&o)
	o.maxBytes = 40
	_, err := listDirectory(context.Background(), base, &o, listArgs{})
	require.ErrorIs(t, err, toolsy.ErrValidation)
	o.maxBytes = 1000
	o.maxNameBytes = 3
	_, err = listDirectory(context.Background(), base, &o, listArgs{})
	require.ErrorIs(t, err, toolsy.ErrValidation)
	o.maxNameBytes = 255
	o.maxScanEntries = 10
	_, err = listDirectory(context.Background(), base, &o, listArgs{Offset: 9, Limit: 1})
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestReadAndListRootAccessBoundary(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "link")))
	var o options
	applyDefaults(&o)
	for _, path := range []string{"link/secret", "../secret", filepath.Join(outside, "secret")} {
		_, err := doReadFile(context.Background(), base, &o, path)
		require.ErrorIs(t, err, toolsy.ErrValidation)
	}
	_, err := doListDir(context.Background(), base, &o, "link")
	require.ErrorIs(t, err, toolsy.ErrValidation)
}
