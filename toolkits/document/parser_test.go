package document

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCSV_CancelDuringRows(t *testing.T) {
	t.Parallel()
	var rows [][]string
	for range 5000 {
		rows = append(rows, []string{"col", strings.Repeat("x", 64)})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := rowsToMarkdownTable(ctx, rows, 1<<20)
	require.NotContains(t, got, "[Truncated]")
}

func TestParseDOCX_CancelDuringZipScan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "many.docx")
	f, err := os.Create(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for i := range 2000 {
		hdr := &zip.FileHeader{Name: filepath.Join("unused", string(rune('a'+i%26))+".xml"), Method: zip.Store}
		entry, createErr := w.CreateHeader(hdr)
		require.NoError(t, createErr)
		_, _ = entry.Write([]byte("<x/>"))
	}
	body := []byte(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>late</w:t></w:r></w:p></w:body></w:document>`,
	)
	entry, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = entry.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rf, err := os.Open(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	defer func() { _ = rf.Close() }()
	info, err := rf.Stat()
	require.NoError(t, err)
	_, err = parseDOCX(ctx, rf, info.Size(), 1<<20)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParsePDF_CancelBeforeRead(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := parsePDF(ctx, "/nonexistent/file.pdf", 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
