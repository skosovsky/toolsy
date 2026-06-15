package document

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func decodeExtractResult(t *testing.T, c toolsy.Chunk) ExtractWireResult {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out ExtractWireResult
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestExtractCSV_Success(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("a,b\n1,2\n3,4"), 0o600))

	tool, err := AsTool()
	require.NoError(t, err)

	var result ExtractWireResult
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeExtractResult(t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Text, "a")
	require.Contains(t, result.Text, "b")
	require.Contains(t, result.Text, "1")
}

func TestExtractCSV_MultilineCellNormalized(t *testing.T) {
	dir := t.TempDir()
	// CSV with quoted multiline cell: newlines should be normalized to space in Markdown output
	csvPath := filepath.Join(dir, "data.csv")
	content := "name,note\nAlice,\"line1\nline2\"\nBob,single"
	require.NoError(t, os.WriteFile(csvPath, []byte(content), 0o600))

	tool, err := AsTool()
	require.NoError(t, err)

	var result ExtractWireResult
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeExtractResult(t, c)
				return nil
			},
		),
	)
	// Multiline cell should be normalized to "line1 line2" so table does not break
	require.Contains(t, result.Text, "line1 line2")
	require.Contains(t, result.Text, "Alice")
	require.Contains(t, result.Text, "Bob")
}

func TestExtract_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.xyz")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))

	tool, err := AsTool()
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "unsupported")
}

func TestExtract_URLDisabled(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"https://example.com/file.pdf"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "URL fetch is disabled")
}

func TestExtract_EmptyArgs(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestAsTool_ReturnsOneTool(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)
	require.NotNil(t, tool)
	require.Equal(t, "document_extract_text", tool.Manifest().Name)
}

func TestAsTool_DefaultReadOnlyManifest(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)
	require.True(t, tool.Manifest().ReadOnly)
}

func TestAsTool_AllowRemoteClearsReadOnlyManifest(t *testing.T) {
	tool, err := AsTool(WithAllowRemote(true))
	require.NoError(t, err)
	require.False(t, tool.Manifest().ReadOnly)
}

func TestExtract_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "big.csv")
	// Create file larger than maxBytes (1 MB limit)
	large := make([]byte, 1024*1024+1)
	for i := range large {
		large[i] = 'x'
	}
	require.NoError(t, os.WriteFile(csvPath, large, 0o600))

	tool, err := AsTool(WithMaxBytes(1024 * 1024))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(contentByteCap(1024*1024)))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestExtract_CSVTableExceedsCap(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "wide.csv")
	body := "col1,col2,col3,col4,col5,col6,col7,col8\nv1,v2,v3,v4,v5,v6,v7,v8"
	require.NoError(t, os.WriteFile(csvPath, []byte(body), 0o600))

	const maxBytes = 40
	contentCap := contentByteCap(maxBytes)
	tool, err := AsTool(WithMaxBytes(maxBytes))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
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

func TestExtract_LocalFileBetweenWireAndContentCap(t *testing.T) {
	const wireMax = 1000
	contentCap := contentByteCap(wireMax)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "edge.csv")
	// Size between content cap and wire max would pass old wire-only stat check.
	require.NoError(t, os.WriteFile(csvPath, []byte(strings.Repeat("x", contentCap+1)), 0o600))

	tool, err := AsTool(WithMaxBytes(wireMax))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
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

// minimalDOCX creates a minimal valid .docx (ZIP with word/document.xml containing one paragraph).
func minimalDOCX(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "minimal.docx")
	f, err := os.Create(path) // #nosec G304 -- path from t.TempDir()
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	w := zip.NewWriter(f)
	body := []byte(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello DOCX</w:t></w:r></w:p></w:body></w:document>`,
	)
	docW, err := w.Create("word/document.xml")
	require.NoError(t, err)
	_, err = docW.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return path
}

func TestExtract_DOCX_Success(t *testing.T) {
	dir := t.TempDir()
	docxPath := minimalDOCX(t, dir)

	tool, err := AsTool()
	require.NoError(t, err)

	var result ExtractWireResult
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + docxPath + `"}`)},
			func(c toolsy.Chunk) error {
				result = decodeExtractResult(t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Text, "Hello DOCX")
}

func TestExtract_DOCX_UncompressedExceeds(t *testing.T) {
	dir := t.TempDir()
	docxPath := minimalDOCX(t, dir)

	zr, err := zip.OpenReader(docxPath)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	var uncompressed uint64
	for _, f := range zr.File {
		if f.Name == wordDocXML {
			uncompressed = f.UncompressedSize64
			break
		}
	}
	require.Positive(t, uncompressed)

	f, err := os.Open(docxPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	require.NoError(t, err)

	_, err = parseDOCX(context.Background(), f, info.Size(), int(uncompressed-1))
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "docx uncompressed")
}

func TestExtract_CancelDuringParse(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("a,b\n1,2\n3,4"), 0o600))

	tool, err := AsTool()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = tool.Execute(
		ctx,
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	if te, ok := toolsy.AsToolError(err); ok {
		require.NotEqual(t, toolsy.CodeInternal, te.Code)
	}
}

func TestExtract_Remote_SSRFBlocked(t *testing.T) {
	tool, err := AsTool(WithAllowRemote(true))
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"http://127.0.0.1:9999/file.pdf"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "private or loopback")
}

func TestExtract_Remote_ExceedsMaxBytes(t *testing.T) {
	const maxBytes = 1024
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a\n" + strings.Repeat("x", maxBytes+100)))
	}))
	defer server.Close()

	tool, err := AsTool(
		WithAllowRemote(true),
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxBytes(maxBytes),
	)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/big.csv"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, strconv.Itoa(contentByteCap(maxBytes)))
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.ErrorIs(t, err, toolsy.ErrValidation)
}

func TestExtract_Remote_BetweenWireAndContentCap(t *testing.T) {
	const wireMax = 1000
	contentCap := contentByteCap(wireMax)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(strings.Repeat("x", contentCap+1)))
	}))
	defer server.Close()

	tool, err := AsTool(
		WithAllowRemote(true),
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxBytes(wireMax),
	)
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/edge.csv"}`)},
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

func TestLocalFilePathForExtract_CanceledBeforeStat_ReturnsInternal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	_, _, err := localFilePathForExtract(ctx, path, &options{maxBytes: 1024})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
}

func TestCopyRemote_CancelOverReadLimit_InterruptWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := &options{maxBytes: 64, allowRemote: true}
	applyDefaults(o)
	contentCap := contentByteCap(o.maxBytes)
	huge := strings.Repeat("x", contentCap+128)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(huge)),
		Header:     http.Header{"Content-Type": []string{"text/csv"}},
	}
	_, _, err := copyRemoteResponseToTemp(ctx, resp, "http://example.com/file.csv", o)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestExtract_Remote_CancelDuringDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(
		WithAllowRemote(true),
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- tool.Execute(
			ctx,
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `"}`)},
			func(toolsy.Chunk) error { return nil },
		)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err = <-done
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestExtract_Remote_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("col1,col2\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result ExtractWireResult
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/data.csv"}`)},
			func(c toolsy.Chunk) error {
				result = decodeExtractResult(t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Text, "col1")
	require.Contains(t, result.Text, "1")
}

func TestExtract_Remote_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/data.csv"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Contains(t, te.Reason, "500")
}

func TestExtract_Remote_QueryStringURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result ExtractWireResult
	// URL with query string: format should be taken from path (.csv), not from ?sig=...
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/file.csv?sig=abc"}`)},
			func(c toolsy.Chunk) error {
				result = decodeExtractResult(t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Text, "a")
	require.Contains(t, result.Text, "1")
}

// TestExtract_Remote_RedirectToLoopbackBlocked ensures redirect to loopback is rejected when private IPs are disallowed.
func TestExtract_Remote_RedirectToLoopbackBlocked(t *testing.T) {
	fn := httptool.CheckRedirectRemote(false, nil)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:9999/file.csv", nil)
	require.NoError(t, err)
	err = fn(req, []*http.Request{{URL: req.URL}})
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "private or loopback")
}

func TestExtract_Remote_Redirect_AllowsLoopbackWhenPrivateAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:99999/file.csv", http.StatusFound)
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/doc.csv"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err) // connection fails, but redirect validation passes with allowPrivateIPs
}

func TestAsTool_WithResultFormatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(WithResultFormatter(func(res ExtractWireResult) (any, error) {
		return map[string]int{"len": len(res.Text)}, nil
	}))
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
			func(c toolsy.Chunk) error {
				require.NoError(t, json.Unmarshal(c.Data, &payload))
				return nil
			},
		),
	)
	require.Positive(t, payload["len"])
}

func TestAsTool_WithMaxBytes_WithResultFormatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(
		WithMaxBytes(50),
		WithResultFormatter(func(_ ExtractWireResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500)}, nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 50+len(textprocessor.TruncationSuffix)+2)
}

func TestAsTool_RemoteURL_WithFormatterAndValidator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(
		WithAllowRemote(true),
		WithHTTPClient(server.Client()),
		WithAllowPrivateIPs(true),
		WithMaxBytes(50),
		WithResultFormatter(func(res ExtractWireResult) (any, error) {
			return map[string]int{"len": len(res.Text)}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]int)
			if !ok || payload["len"] <= 0 {
				return errors.New("invalid payload")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"url":"` + server.URL + `/data.csv"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 50+len(textprocessor.TruncationSuffix)+2)
}

func TestExtractCSV_WireCapSingleTruncSuffix(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "wide.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("text\n"+strings.Repeat("x", 220)), 0o600))

	tool, err := AsTool(WithMaxBytes(250))
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + csvPath + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 250+len(textprocessor.TruncationSuffix)+2)
	require.LessOrEqual(t, strings.Count(string(wire), "[Truncated]"), 1)
	var payload ExtractWireResult
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.NotContains(t, payload.Text, "[Truncated]")
}

func TestAsTool_TripleIoC_MaxBytesFormatterValidator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(
		WithMaxBytes(80),
		WithResultFormatter(func(res ExtractWireResult) (any, error) {
			return map[string]string{"blob": strings.Repeat("z", 500) + res.Text}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]string)
			if !ok || payload["blob"] == "" {
				return errors.New("invalid payload")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var wire []byte
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
			func(c toolsy.Chunk) error {
				wire = append([]byte(nil), c.Data...)
				return nil
			},
		),
	)
	require.LessOrEqual(t, len(wire), 80+len(textprocessor.TruncationSuffix)+2)
}

func TestAsTool_WithHostResultValidator_Reject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(WithHostResultValidator(func(_ any) error {
		return assert.AnError
	}))
	require.NoError(t, err)
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
}

func TestAsTool_WithHostResultValidator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(WithHostResultValidator(func(v any) error {
		_, ok := v.(ExtractWireResult)
		if !ok {
			return assert.AnError
		}
		return nil
	}))
	require.NoError(t, err)
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestAsTool_FormatterAndValidator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2"), 0o600))

	tool, err := AsTool(
		WithResultFormatter(func(res ExtractWireResult) (any, error) {
			return map[string]int{"len": len(res.Text)}, nil
		}),
		WithHostResultValidator(func(v any) error {
			payload, ok := v.(map[string]int)
			if !ok {
				return errors.New("expected formatter output map")
			}
			if payload["len"] <= 0 {
				return errors.New("empty text")
			}
			return nil
		}),
	)
	require.NoError(t, err)
	var payload map[string]int
	require.NoError(
		t,
		tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"` + path + `"}`)},
			func(c toolsy.Chunk) error {
				return json.Unmarshal(c.Data, &payload)
			},
		),
	)
	require.Positive(t, payload["len"])
}
