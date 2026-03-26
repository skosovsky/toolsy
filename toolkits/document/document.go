package document

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/textutil"
)

const maxRedirects = 10

type extractArgs struct {
	FilePath string `json:"file_path,omitempty"`
	URL      string `json:"url,omitempty"`
}

type extractResult struct {
	Text string `json:"text"`
}

// AsTool returns a single tool that extracts text from PDF, CSV, or DOCX (by file path or URL).
func AsTool(opts ...Option) (toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	tool, err := toolsy.NewTool[extractArgs, extractResult](
		o.toolName,
		o.toolDesc,
		func(ctx context.Context, _ toolsy.RunContext, args extractArgs) (extractResult, error) {
			return doExtract(ctx, &o, args.FilePath, args.URL)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/document: build tool: %w", err)
	}
	return tool, nil
}

func doExtract(ctx context.Context, o *options, filePath, url string) (extractResult, error) {
	if filePath == "" && url == "" {
		return extractResult{}, &toolsy.ClientError{
			Reason:    "file_path or url is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	if filePath != "" && url != "" {
		return extractResult{}, &toolsy.ClientError{
			Reason:    "provide either file_path or url, not both",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	if url != "" && !o.allowRemote {
		return extractResult{}, &toolsy.ClientError{
			Reason:    "URL fetch is disabled (use WithAllowRemote(true))",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}

	var path string
	var format string
	var err error
	if url != "" {
		path, format, err = fetchRemoteToTemp(ctx, o, url)
		if err != nil {
			return extractResult{}, err
		}
		defer func() { _ = os.Remove(path) }()
	} else {
		path = filepath.Clean(filePath)
		var info os.FileInfo
		info, err = os.Stat(path)
		if err != nil {
			return extractResult{}, fmt.Errorf("toolkit/document: stat: %w", err)
		}
		if info.Size() > int64(o.maxBytes) {
			return extractResult{}, &toolsy.ClientError{
				Reason:    "file too large",
				Retryable: false,
				Err:       toolsy.ErrValidation,
			}
		}
		format = formatFromURL(path)
	}

	format = strings.ToLower(strings.TrimPrefix(format, "."))
	text, err := extractTextByFormat(path, format, o)
	if err != nil {
		return extractResult{}, err
	}
	// Final truncation (UTF-8 safe)
	if len(text) > o.maxBytes {
		text = textutil.TruncateStringUTF8(text, o.maxBytes, truncateSuffix)
	}
	return extractResult{Text: text}, nil
}

// extractTextByFormat parses the file at path according to format (csv, pdf, docx).
func extractTextByFormat(path, format string, o *options) (string, error) {
	switch format {
	case "csv":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return "", openErr
		}
		defer func() { _ = f.Close() }()
		text, err := parseCSV(f, o.maxBytes)
		if err != nil {
			return "", fmt.Errorf("toolkit/document: parse: %w", err)
		}
		return text, nil
	case "pdf":
		text, err := parsePDF(path, o.maxBytes)
		if err != nil {
			return "", fmt.Errorf("toolkit/document: parse: %w", err)
		}
		return text, nil
	case "docx":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return "", openErr
		}
		defer func() { _ = f.Close() }()
		info, statErr := f.Stat()
		if statErr != nil {
			return "", fmt.Errorf("toolkit/document: stat docx file: %w", statErr)
		}
		text, err := parseDOCX(f, info.Size(), o.maxBytes)
		if err != nil {
			return "", fmt.Errorf("toolkit/document: parse: %w", err)
		}
		return text, nil
	default:
		return "", &toolsy.ClientError{
			Reason:    "unsupported format: " + format,
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
}

// fetchRemoteToTemp downloads a remote document to a temp file and returns path and detected format.
func fetchRemoteToTemp(ctx context.Context, o *options, rawURL string) (string, string, error) {
	if err := validateRemoteURL(ctx, rawURL, o.allowPrivateIPs); err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("toolkit/document: request: %w", err)
	}
	client := *o.httpClient
	client.CheckRedirect = func(redirectReq *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return &toolsy.ClientError{
				Reason:    "too many redirects",
				Retryable: false,
				Err:       toolsy.ErrValidation,
			}
		}
		if verr := validateRemoteURL(redirectReq.Context(), redirectReq.URL.String(), false); verr != nil {
			return verr
		}
		return nil
	}
	resp, err := client.Do(req) // #nosec G704 -- URL validated; redirects validated in CheckRedirect
	if err != nil {
		if toolsy.IsClientError(err) {
			return "", "", err
		}
		return "", "", fmt.Errorf("toolkit/document: fetch: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", &toolsy.ClientError{
			Reason:    "remote file fetch failed: " + resp.Status,
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	format := formatFromURL(rawURL)
	if format == "" {
		format = formatFromContentType(resp.Header.Get("Content-Type"))
	}
	tmp, err := os.CreateTemp("", "document-*")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmp.Name()
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, int64(o.maxBytes)+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	if n > int64(o.maxBytes) {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", "", &toolsy.ClientError{
			Reason:    "remote file too large",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", closeErr
	}
	return tmpPath, format, nil
}

func formatFromURL(u string) string {
	if strings.Contains(u, "://") {
		if parsed, err := url.Parse(u); err == nil && parsed.Path != "" {
			ext := filepath.Ext(parsed.Path)
			return strings.TrimPrefix(strings.ToLower(ext), ".")
		}
	}
	ext := filepath.Ext(u)
	return strings.TrimPrefix(strings.ToLower(ext), ".")
}

func formatFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	if strings.Contains(ct, "pdf") {
		return "pdf"
	}
	if strings.Contains(ct, "csv") {
		return "csv"
	}
	if strings.Contains(ct, "wordprocessingml") || strings.Contains(ct, "vnd.openxmlformats") {
		return "docx"
	}
	return ""
}

// validateRemoteURL checks scheme (http/https), host presence, and blocks private/loopback IPs unless allowPrivateIPs.
func validateRemoteURL(ctx context.Context, rawURL string, allowPrivateIPs bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return &toolsy.ClientError{
			Reason:    "invalid URL: " + err.Error(),
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return &toolsy.ClientError{
			Reason:    "only http and https schemes are allowed",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return &toolsy.ClientError{
			Reason:    "URL host is missing",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	if allowPrivateIPs {
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return &toolsy.ClientError{
			Reason:    "SSRF: host lookup failed: " + err.Error(),
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	if slices.ContainsFunc(addrs, func(a net.IPAddr) bool { return isPrivateIP(a.IP) }) {
		return &toolsy.ClientError{
			Reason:    "SSRF: private or loopback IP not allowed",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}
