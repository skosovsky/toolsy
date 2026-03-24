package sandboxfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "nested", input: "dir/file.txt", want: "dir/file.txt"},
		{name: "backslashes", input: `dir\file.txt`, want: "dir/file.txt"},
		{name: "empty", input: "", wantErr: true},
		{name: "absolute", input: "/tmp/file.txt", wantErr: true},
		{name: "windows absolute slash", input: `C:/temp/file.txt`, wantErr: true},
		{name: "windows absolute backslash", input: `C:\temp\file.txt`, wantErr: true},
		{name: "unc slash", input: `//server/share/file.txt`, wantErr: true},
		{name: "unc backslash", input: `\\server\share\file.txt`, wantErr: true},
		{name: "traversal", input: "../secret.txt", wantErr: true},
		{name: "dot", input: ".", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRelativePath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWriteWorkspace(t *testing.T) {
	root := t.TempDir()
	err := WriteWorkspace(root, map[string][]byte{
		"dir/data.txt": []byte("hello"),
		"root.txt":     []byte("world"),
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, "dir", "data.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	data, err = os.ReadFile(filepath.Join(root, "root.txt"))
	require.NoError(t, err)
	require.Equal(t, "world", string(data))
}

func TestWriteWorkspaceRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	err := WriteWorkspace(root, map[string][]byte{
		"../secret.txt": []byte("nope"),
	})
	require.Error(t, err)
}

func TestCanonicalizeFilesNormalizesPaths(t *testing.T) {
	files, err := CanonicalizeFiles(map[string][]byte{
		`dir\file.txt`: []byte("hello"),
	})
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"dir/file.txt": []byte("hello")}, files)
}

func TestCanonicalizeFilesRejectsCollisionsAfterNormalization(t *testing.T) {
	_, err := CanonicalizeFiles(map[string][]byte{
		"data.txt":        []byte("one"),
		"dir/../data.txt": []byte("two"),
	})
	require.Error(t, err)
}

func TestCanonicalizeFilesRejectsReservedPaths(t *testing.T) {
	_, err := CanonicalizeFiles(map[string][]byte{
		"dir/../main.py": []byte("print(1)"),
	}, "main.py")
	require.Error(t, err)
}

func TestCanonicalizeFilesRejectsInvalidReservedPathWhenFilesEmpty(t *testing.T) {
	_, err := CanonicalizeFiles(nil, "../main.py")
	require.Error(t, err)
}
