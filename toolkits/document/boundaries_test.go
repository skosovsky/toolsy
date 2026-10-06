package document

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func executeReference(t *testing.T, tool toolsy.Tool, ref string) ([]byte, error) {
	t.Helper()
	args, err := json.Marshal(map[string]string{"file_path": ref})
	require.NoError(t, err)
	var wire []byte
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: args},
		func(c toolsy.Chunk) error {
			wire = append([]byte(nil), c.Data...)
			return nil
		},
	)
	return wire, err
}

func TestLocalAccessRequiresHostBoundary(t *testing.T) {
	// Arrange.
	path := filepath.Join(t.TempDir(), "secret.csv")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o600))
	tool, err := AsTool()
	require.NoError(t, err)
	// Act.
	wire, err := executeReference(t, tool, path)
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Contains(t, err.Error(), "local source is disabled")
	require.Empty(t, wire)
}

func TestLocalRootContainsActualOpenAndRetainsSource(t *testing.T) {
	// Arrange.
	dir, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("name\nАнна"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.csv"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link")))
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	tool, err := AsTool(WithLocalRoot(root))
	require.NoError(t, err)
	// Act / Assert.
	for _, ref := range []string{"link/secret.csv", "../secret.csv", filepath.Join(outside, "secret.csv")} {
		wire, executeErr := executeReference(t, tool, ref)
		require.Error(t, executeErr)
		require.Empty(t, wire)
	}
	wire, err := executeReference(t, tool, "data.csv")
	require.NoError(t, err)
	var result ExtractWireResult
	require.NoError(t, json.Unmarshal(wire, &result))
	require.Equal(t, "data.csv", result.Source)
	require.Contains(t, result.Text, "Анна")
}

func TestSourcePortEnforcesActualReadAndCloses(t *testing.T) {
	// Arrange.
	closed := false
	tool, err := AsTool(
		WithLimits(Limits{SourceBytes: 32}),
		WithLocalSource(func(context.Context, string) (io.ReadCloser, error) {
			return &trackedReader{Reader: strings.NewReader(strings.Repeat("x", 100)), closed: &closed}, nil
		}),
	)
	require.NoError(t, err)
	// Act.
	wire, err := executeReference(t, tool, "external.csv")
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.True(t, closed)
	require.Empty(t, wire)
}

type trackedReader struct {
	io.Reader

	closed *bool
}

func (r *trackedReader) Close() error { *r.closed = true; return nil }

func TestIndependentCollectionItemAndWireLimits(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		limits     Limits
		wire       int
		reason     string
	}{
		{"rows", "a\nb\nc", Limits{MaxItems: 2}, 2048, "row count"},
		{"cell", "header\n" + strings.Repeat("я", 8), Limits{ItemBytes: 8}, 2048, "cell byte"},
		{"escaped wire", "value\n" + strings.Repeat("<я>", 20), Limits{}, 128, "wire byte"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			tool, err := AsTool(
				WithMaxBytes(tc.wire),
				WithLimits(tc.limits),
				WithLocalSource(func(context.Context, string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(tc.data)), nil
				}),
			)
			require.NoError(t, err)
			// Act.
			wire, err := executeReference(t, tool, "source.csv")
			// Assert.
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.Contains(t, err.Error(), tc.reason)
			require.Empty(t, wire)
		})
	}
}

func TestCompressedDOCXAndForgedDirectoryRejectBeforeExpansion(t *testing.T) {
	// Arrange: tiny compressed archive with very large expanded XML.
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create(wordDocXML)
	require.NoError(t, err)
	_, err = io.WriteString(entry, "<document><t>"+strings.Repeat("x", 1<<20)+"</t></document>")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.Less(t, archive.Len(), 4096)
	// Act.
	_, err = parseDOCXWithLimits(
		context.Background(),
		bytes.NewReader(archive.Bytes()),
		int64(archive.Len()),
		Limits{SourceBytes: 4096, ParsedBytes: 128, MaxItems: 4, ItemBytes: 64},
	)
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Contains(t, err.Error(), "uncompressed")

	// Arrange: advertised EOCD entry count lies; actual metadata must be counted before zip.NewReader.
	data := append([]byte(nil), archive.Bytes()...)
	binary.LittleEndian.PutUint16(data[len(data)-22+10:], 0)
	// Act.
	_, err = parseDOCXWithLimits(
		context.Background(),
		bytes.NewReader(data),
		int64(len(data)),
		Limits{SourceBytes: 4096, ParsedBytes: 128, MaxItems: 4, ItemBytes: 64},
	)
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Contains(t, err.Error(), "count mismatch")
}

func TestHostilePDFRejectedBeforeInProcessDecode(t *testing.T) {
	// Arrange: compressed payload much larger than its source. Default must never enter PDF parser.
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, err := io.WriteString(zw, strings.Repeat("A", 8<<20))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	data := append([]byte("%PDF-1.4\n/Filter /FlateDecode\nstream\n"), compressed.Bytes()...)
	tool, err := AsTool(
		WithLocalSource(
			func(context.Context, string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
		),
	)
	require.NoError(t, err)
	// Act.
	wire, err := executeReference(t, tool, "hostile.pdf")
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Contains(t, err.Error(), "page decode allocations cannot be bounded")
	require.Empty(t, wire)
}

func TestNegativeLimitsFailConstructionAndZeroUsesFiniteDefaults(t *testing.T) {
	// Arrange / Act / Assert.
	for _, option := range []Option{WithMaxBytes(-1), WithLimits(Limits{SourceBytes: -1}), WithLimits(Limits{ParsedBytes: -1}), WithLimits(Limits{MaxItems: -1}), WithLimits(Limits{ItemBytes: -1})} {
		_, err := AsTool(option)
		require.Error(t, err)
	}
	limits := parserLimits(&options{})
	require.Positive(t, limits.SourceBytes)
	require.Positive(t, limits.ParsedBytes)
	require.Positive(t, limits.MaxItems)
	require.Positive(t, limits.ItemBytes)
}

func TestDOCXMissingClosingTextNodeRejects(t *testing.T) {
	// Arrange.
	malformed := []byte("<document><t>")
	// Act.
	_, err := extractTextFromWordXML(context.Background(), malformed, 1024)
	// Assert.
	require.Error(t, err)
}

func TestDOCXTextNodePreservesCommentSplitAndChecksCombinedBudget(t *testing.T) {
	// Arrange.
	raw := []byte("<document><t>ab<!--comment-->cd</t></document>")
	// Act.
	text, err := extractTextFromWordXMLWithLimits(context.Background(), raw, Limits{ParsedBytes: 128, ItemBytes: 8})
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "abcd", text)
	// Act.
	_, err = extractTextFromWordXMLWithLimits(context.Background(), raw, Limits{ParsedBytes: 128, ItemBytes: 3})
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestExplicitPDFOptInEnforcesPageTextBudget(t *testing.T) {
	// Arrange.
	dir := t.TempDir()
	path := filepath.Join(dir, "small.pdf")
	writeMinimalPDFWithText(t, path, "hello PDF")
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	tool, err := AsTool(WithLocalRoot(root), WithInProcessPDF(true))
	require.NoError(t, err)
	// Act.
	wire, err := executeReference(t, tool, "small.pdf")
	// Assert.
	require.NoError(t, err)
	var result ExtractWireResult
	require.NoError(t, json.Unmarshal(wire, &result))
	require.Contains(t, result.Text, "hello PDF")
	require.Equal(t, "small.pdf", result.Source)
	// Arrange.
	bounded, err := AsTool(WithLocalRoot(root), WithInProcessPDF(true), WithLimits(Limits{ItemBytes: 1}))
	require.NoError(t, err)
	// Act.
	wire, err = executeReference(t, bounded, "small.pdf")
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrValidation)
	require.Contains(t, err.Error(), "page text byte")
	require.Empty(t, wire)
}
