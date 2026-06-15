package document

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestParseCSV_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"read: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	mapped := toolsy.MapToolkitReadError(
		context.Background(),
		composite,
		"document: csv read",
		1<<20,
		"csv",
		"",
	)
	require.Error(t, mapped)
	require.ErrorIs(t, mapped, context.Canceled)
	te, ok := toolsy.AsToolError(mapped)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestParseCSV_CancelDuringRows(t *testing.T) {
	t.Parallel()
	var rows [][]string
	for range 5000 {
		rows = append(rows, []string{"col", strings.Repeat("x", 64)})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := rowsToMarkdownTable(ctx, rows)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, got)
}

func TestParseDOCX_CancelDuringZipScan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "many.docx")
	f, err := os.Create(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for i := range 100 {
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

func TestParseDOCX_ExceedsByteLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "large.docx")
	f, err := os.Create(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	w := zip.NewWriter(f)
	text := strings.Repeat("x", 4096)
	body := []byte(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` +
			text + `</w:t></w:r></w:p></w:body></w:document>`,
	)
	entry, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = entry.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	rf, err := os.Open(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	defer func() { _ = rf.Close() }()
	info, err := rf.Stat()
	require.NoError(t, err)

	const maxBytes = 1024
	_, err = parseDOCX(context.Background(), rf, info.Size(), maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "1024")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseDOCX_CancelOverCap_InterruptWins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "large.docx")
	f, err := os.Create(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	w := zip.NewWriter(f)
	text := strings.Repeat("x", 4096)
	body := []byte(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` +
			text + `</w:t></w:r></w:p></w:body></w:document>`,
	)
	entry, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = entry.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const maxBytes = 1024
	_, err = extractTextByFormat(ctx, path, "docx", &options{maxBytes: maxBytes})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestParseCSV_ExceedsByteLimit(t *testing.T) {
	t.Parallel()
	const maxBytes = 64
	body := "a,b\n" + strings.Repeat("x", maxBytes+100)
	_, err := parseCSV(context.Background(), strings.NewReader(body), maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "64")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseDOCX_TooManyZipEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "many.docx")
	f, err := os.Create(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for i := range maxZipEntries + 1 {
		hdr := &zip.FileHeader{Name: fmt.Sprintf("entry%d.xml", i), Method: zip.Store}
		entry, createErr := w.CreateHeader(hdr)
		require.NoError(t, createErr)
		_, _ = entry.Write([]byte("<x/>"))
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	rf, err := os.Open(path) // #nosec G304 -- test temp dir
	require.NoError(t, err)
	defer func() { _ = rf.Close() }()
	info, err := rf.Stat()
	require.NoError(t, err)
	_, err = parseDOCX(context.Background(), rf, info.Size(), 1<<20)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(maxZipEntries))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseCSV_TableExceedsByteLimit(t *testing.T) {
	t.Parallel()
	const maxBytes = 40
	body := "col1,col2,col3,col4,col5,col6,col7,col8\nv1,v2,v3,v4,v5,v6,v7,v8"
	_, err := parseCSV(context.Background(), strings.NewReader(body), maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "40")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseDOCX_ExtractedTextExpansionExceedsLimit(t *testing.T) {
	t.Parallel()
	const maxBytes = 24
	var paragraphs strings.Builder
	for range 30 {
		paragraphs.WriteString("<w:p><w:r><w:t>a</w:t></w:r></w:p>")
	}
	body := []byte(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
			paragraphs.String() + `</w:body></w:document>`,
	)
	_, err := extractTextFromWordXML(context.Background(), body, maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(maxBytes))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func writeMinimalPDFWithText(t *testing.T, path, text string) {
	t.Helper()
	esc := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `(`, `\(`)
		s = strings.ReplaceAll(s, `)`, `\)`)
		return s
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", esc(text))
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var offsets []int
	addObj := func(id int, body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	addObj(1, "<</Type/Catalog/Pages 2 0 R>>")
	addObj(2, "<</Type/Pages/Kids[3 0 R]/Count 1>>")
	addObj(3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 4 0 R>>>>/Contents 5 0 R>>")
	addObj(4, "<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>")
	addObj(5, fmt.Sprintf("<</Length %d>>stream\n%s\nendstream", len(stream), stream))
	xrefPos := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(offsets)+1)
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", len(offsets)+1, xrefPos)
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
}

func TestParsePDF_ExceedsByteLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "large.pdf")
	writeMinimalPDFWithText(t, path, strings.Repeat("A", 4096))

	const maxBytes = 64
	_, err := parsePDF(context.Background(), path, maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "64")
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParseCSV_CancelOverTableCap_ReturnsInternal(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Table from minimal CSV exceeds tiny cap; cancel must win over validation.
	body := strings.NewReader("a,b,c,d,e,f,g,h\n1,2,3,4,5,6,7,8")
	_, err := parseCSV(ctx, body, 20)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestParsePDF_CanceledBeforeStat_ReturnsInternal(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	writeMinimalPDFWithText(t, path, "hello")
	_, err := parsePDF(ctx, path, 1024)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestParsePDF_FileSizeExceedsLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.bin")
	const maxBytes = 64
	require.NoError(t, os.WriteFile(path, make([]byte, maxBytes+1), 0o600))

	_, err := parsePDF(context.Background(), path, maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "pdf file exceeds")
	require.Contains(t, te.Reason, strconv.Itoa(maxBytes))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestParsePDF_SingleOversizedPage_FailClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "one-page.pdf")
	writeMinimalPDFWithText(t, path, strings.Repeat("A", 4096))

	const maxBytes = 64
	_, err := parsePDF(context.Background(), path, maxBytes)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(maxBytes))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}
