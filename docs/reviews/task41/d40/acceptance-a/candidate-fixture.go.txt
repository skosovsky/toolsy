package fstool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestPublicHardlinkAliasRetainsExistingInode(t *testing.T) {
	// Arrange: both names refer to a disposable file; no real host data is used.
	parent := t.TempDir()
	base := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(base, 0o750))
	outside := filepath.Join(parent, "outside")
	inside := filepath.Join(base, "alias")
	require.NoError(t, os.WriteFile(outside, []byte("outside bytes"), 0o600))
	require.NoError(t, os.Link(outside, inside))
	before, err := os.Stat(outside)
	require.NoError(t, err)
	tools, err := AsTools(base)
	require.NoError(t, err)
	var read readResult
	// Act: path containment permits the regular-file hardlink.
	err = tools[1].Execute(context.Background(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"alias"}`)},
		func(c toolsy.Chunk) error { read = decodeJSONChunk[readResult](t, c); return nil })
	// Assert: this is a documented limit, not an isolation guarantee.
	require.NoError(t, err)
	require.Equal(t, "outside bytes", read.Content)
	// Act: update through the public write tool.
	err = tools[2].Execute(context.Background(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"alias","content":"new"}`)},
		func(toolsy.Chunk) error { return nil })
	// Assert: exact shorter content replaces bytes in the same inode, including the outside alias.
	require.NoError(t, err)
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "new", string(data))
	after, err := os.Stat(inside)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "write is an in-place update, not replacement")
}

func TestPublicReadRangesDoNotPinSnapshot(t *testing.T) {
	// Arrange.
	base := t.TempDir()
	path := filepath.Join(base, "file")
	require.NoError(t, os.WriteFile(path, []byte("abcdef"), 0o600))
	tools, err := AsTools(base)
	require.NoError(t, err)
	var first, second readResult
	// Act: the host changes the same file between range calls.
	err = tools[1].Execute(context.Background(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"file","length":3}`)},
		func(c toolsy.Chunk) error { first = decodeJSONChunk[readResult](t, c); return nil })
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("UVWXYZ"), 0o600))
	err = tools[1].Execute(context.Background(), toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"path":"file","offset":3,"length":3}`)},
		func(c toolsy.Chunk) error { second = decodeJSONChunk[readResult](t, c); return nil })
	// Assert: offsets do not identify an immutable version.
	require.NoError(t, err)
	require.Equal(t, "abc", first.Content)
	require.Equal(t, int64(3), first.NextOffset)
	require.True(t, first.HasMore)
	require.Equal(t, "XYZ", second.Content)
	require.False(t, second.HasMore)
}
