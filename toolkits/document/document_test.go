package document

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	require.Contains(t, te.Reason, "too large")
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
