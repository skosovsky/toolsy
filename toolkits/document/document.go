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
		func(ctx context.Context, args extractArgs) (extractResult, error) {
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
		return extractResult{}, &toolsy.ClientError{Reason: "file_path or url is required", Err: toolsy.ErrValidation}
	}
	if filePath != "" && url != "" {
		return extractResult{}, &toolsy.ClientError{Reason: "provide either file_path or url, not both", Err: toolsy.ErrValidation}
	}
	if url != "" && !o.allowRemote {
		return extractResult{}, &toolsy.ClientError{Reason: "URL fetch is disabled (use WithAllowRemote(true))", Err: toolsy.ErrValidation}
	}

	var path string
	var format string
	if url != "" {
		if err := validateRemoteURL(url, o.allowPrivateIPs); err != nil {
			return extractResult{}, err
		}
		// Download with size limit (zip bomb / OOM protection)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return extractResult{}, fmt.Errorf("toolkit/document: request: %w", err)
		}
		// Use a client that validates redirects to prevent SSRF via redirect to loopback/private IP
		client := *o.httpClient
		client.CheckRedirect = func(redirectReq *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return &toolsy.ClientError{Reason: "too many redirects", Err: toolsy.ErrValidation}
			}
			// Always reject redirect to private/loopback (SSRF); allowPrivateIPs applies only to initial URL
			if err := validateRemoteURL(redirectReq.URL.String(), false); err != nil {
				return err
			}
			return nil
		}
		resp, err := client.Do(req) // #nosec G704 -- URL validated; redirects validated in CheckRedirect
		if err != nil {
			if toolsy.IsClientError(err) {
				return extractResult{}, err
			}
			return extractResult{}, fmt.Errorf("toolkit/document: fetch: %w", err)
		}
		defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return extractResult{}, &toolsy.ClientError{Reason: "remote file fetch failed: " + resp.Status, Err: toolsy.ErrValidation}
		}
		format = formatFromURL(url)
		if format == "" {
			format = formatFromContentType(resp.Header.Get("Content-Type"))
		}
		tmp, err := os.CreateTemp("", "document-*")
		if err != nil {
			return extractResult{}, err
		}
		defer func() { _ = os.Remove(tmp.Name()); _ = tmp.Close() }() // #nosec G703 -- tmp path from os.CreateTemp
		n, err := io.Copy(tmp, io.LimitReader(resp.Body, int64(o.maxBytes)+1))
		if err != nil {
			return extractResult{}, err
		}
		if n > int64(o.maxBytes) {
			return extractResult{}, &toolsy.ClientError{Reason: "remote file too large", Err: toolsy.ErrValidation}
		}
		path = tmp.Name()
		_, _ = tmp.Seek(0, 0)
	} else {
		path = filepath.Clean(filePath)
		info, err := os.Stat(path)
		if err != nil {
			return extractResult{}, fmt.Errorf("toolkit/document: stat: %w", err)
		}
		if info.Size() > int64(o.maxBytes) {
			return extractResult{}, &toolsy.ClientError{Reason: "file too large", Err: toolsy.ErrValidation}
		}
		format = formatFromURL(path)
	}

	format = strings.ToLower(strings.TrimPrefix(format, "."))
	var text string
	var err error
	switch format {
	case "csv":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return extractResult{}, openErr
		}
		defer func() { _ = f.Close() }()
		text, err = parseCSV(f, o.maxBytes)
	case "pdf":
		text, err = parsePDF(path, o.maxBytes)
	case "docx":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return extractResult{}, openErr
		}
		defer func() { _ = f.Close() }()
		info, statErr := f.Stat()
		if statErr != nil {
			return extractResult{}, fmt.Errorf("toolkit/document: stat docx file: %w", statErr)
		}
		text, err = parseDOCX(f, info.Size(), o.maxBytes)
	default:
		return extractResult{}, &toolsy.ClientError{Reason: "unsupported format: " + format, Err: toolsy.ErrValidation}
	}
	if err != nil {
		return extractResult{}, fmt.Errorf("toolkit/document: parse: %w", err)
	}
	// Final truncation (UTF-8 safe)
	if len(text) > o.maxBytes {
		text = truncateUTF8(text, o.maxBytes)
	}
	return extractResult{Text: text}, nil
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
func validateRemoteURL(rawURL string, allowPrivateIPs bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return &toolsy.ClientError{Reason: "invalid URL: " + err.Error(), Err: toolsy.ErrValidation}
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return &toolsy.ClientError{Reason: "only http and https schemes are allowed", Err: toolsy.ErrValidation}
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return &toolsy.ClientError{Reason: "URL host is missing", Err: toolsy.ErrValidation}
	}
	if allowPrivateIPs {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return &toolsy.ClientError{Reason: "SSRF: host lookup failed: " + err.Error(), Err: toolsy.ErrValidation}
	}
	if slices.ContainsFunc(ips, isPrivateIP) {
		return &toolsy.ClientError{Reason: "SSRF: private or loopback IP not allowed", Err: toolsy.ErrValidation}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}
