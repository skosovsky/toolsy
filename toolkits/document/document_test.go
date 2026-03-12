package document

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestExtractCSV_Success(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("a,b\n1,2\n3,4"), 0o600))

	tool, err := AsTool()
	require.NoError(t, err)

	var result extractResult
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"file_path":"`+csvPath+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(extractResult); ok {
				result = r
			}
		}
		return nil
	}))
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

	var result extractResult
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"file_path":"`+csvPath+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(extractResult); ok {
				result = r
			}
		}
		return nil
	}))
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

	err = tool.Execute(context.Background(), []byte(`{"file_path":"`+path+`"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "unsupported")
}

func TestExtract_URLDisabled(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)

	err = tool.Execute(context.Background(), []byte(`{"url":"https://example.com/file.pdf"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "URL fetch is disabled")
}

func TestExtract_EmptyArgs(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)

	err = tool.Execute(context.Background(), []byte(`{}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
}

func TestAsTool_ReturnsOneTool(t *testing.T) {
	tool, err := AsTool()
	require.NoError(t, err)
	require.NotNil(t, tool)
	require.Equal(t, "document_extract_text", tool.Name())
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

	err = tool.Execute(context.Background(), []byte(`{"file_path":"`+csvPath+`"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "too large")
}

// minimalDOCX creates a minimal valid .docx (ZIP with word/document.xml containing one paragraph).
func minimalDOCX(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "minimal.docx")
	f, err := os.Create(path) // #nosec G304 -- path from t.TempDir()
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	w := zip.NewWriter(f)
	body := []byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello DOCX</w:t></w:r></w:p></w:body></w:document>`)
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

	var result extractResult
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"file_path":"`+docxPath+`"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(extractResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Contains(t, result.Text, "Hello DOCX")
}

func TestExtract_Remote_SSRFBlocked(t *testing.T) {
	tool, err := AsTool(WithAllowRemote(true))
	require.NoError(t, err)

	err = tool.Execute(context.Background(), []byte(`{"url":"http://127.0.0.1:9999/file.pdf"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "private or loopback")
}

func TestExtract_Remote_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("col1,col2\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result extractResult
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"url":"`+server.URL+`/data.csv"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(extractResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Contains(t, result.Text, "col1")
	require.Contains(t, result.Text, "1")
}

func TestExtract_Remote_QueryStringURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2"))
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)

	var result extractResult
	// URL with query string: format should be taken from path (.csv), not from ?sig=...
	require.NoError(t, tool.Execute(context.Background(), []byte(`{"url":"`+server.URL+`/file.csv?sig=abc"}`), func(c toolsy.Chunk) error {
		if c.RawData != nil {
			if r, ok := c.RawData.(extractResult); ok {
				result = r
			}
		}
		return nil
	}))
	require.Contains(t, result.Text, "a")
	require.Contains(t, result.Text, "1")
}

// TestExtract_Remote_RedirectToLoopbackBlocked ensures redirect to loopback/private IP is rejected (SSRF).
func TestExtract_Remote_RedirectToLoopbackBlocked(t *testing.T) {
	// Server redirects to 127.0.0.1; redirect target is always validated with allowPrivateIPs=false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:99999/file.csv", http.StatusFound)
	}))
	defer server.Close()

	tool, err := AsTool(WithAllowRemote(true), WithHTTPClient(server.Client()), WithAllowPrivateIPs(true))
	require.NoError(t, err)
	// Initial URL is our test server (allowed with WithAllowPrivateIPs); redirect to loopback is still blocked
	err = tool.Execute(context.Background(), []byte(`{"url":"`+server.URL+`/doc.csv"}`), func(toolsy.Chunk) error { return nil })
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "private or loopback")
}
