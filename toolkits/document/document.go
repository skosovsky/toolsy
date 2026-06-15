package document

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/format"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

type extractArgs struct {
	FilePath string `json:"file_path,omitempty"`
	URL      string `json:"url,omitempty"`
}

type ExtractWireResult struct {
	Text string `json:"text"`
}

// AsTool returns a single tool that extracts text from PDF, CSV, or DOCX (by file path or URL).
func AsTool(opts ...Option) (toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	toolOpts := []toolsy.ToolOption{}
	if !o.allowRemote {
		toolOpts = append(toolOpts, toolsy.WithReadOnly())
	}

	if o.resultFormatter != nil || o.hostResultValidator != nil {
		tool, err := toolsy.NewTool[extractArgs, format.JSONResult](
			o.toolName,
			o.toolDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args extractArgs) (format.JSONResult, error) {
				res, extractErr := doExtract(ctx, &o, args.FilePath, args.URL)
				if extractErr != nil {
					return format.JSONResult{}, extractErr
				}
				raw, applyErr := format.ApplyWithEnvelope(
					res,
					func(v ExtractWireResult) ExtractWireResult { return v },
					o.resultFormatter,
					o.hostResultValidator,
					o.maxBytes,
				)
				if applyErr != nil {
					return format.JSONResult{}, applyErr
				}
				return format.JSONResult{Raw: raw}, nil
			},
			toolOpts...,
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/document: build tool: %w", err)
		}
		return tool, nil
	}

	tool, err := toolsy.NewTool[extractArgs, format.JSONResult](
		o.toolName,
		o.toolDesc,
		func(ctx context.Context, _ *toolsy.RunEnv, args extractArgs) (format.JSONResult, error) {
			res, extractErr := doExtract(ctx, &o, args.FilePath, args.URL)
			if extractErr != nil {
				return format.JSONResult{}, extractErr
			}
			return format.ToJSONResult(res, o.maxBytes)
		},
		toolOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/document: build tool: %w", err)
	}
	return tool, nil
}

func doExtract(ctx context.Context, o *options, filePath, url string) (ExtractWireResult, error) {
	if filePath == "" && url == "" {
		return ExtractWireResult{}, toolsy.NewValidationError("file_path or url is required")
	}
	if filePath != "" && url != "" {
		return ExtractWireResult{}, toolsy.NewValidationError("provide either file_path or url, not both")
	}
	if url != "" && !o.allowRemote {
		return ExtractWireResult{}, toolsy.NewValidationError("URL fetch is disabled (use WithAllowRemote(true))")
	}

	var path string
	var format string
	var err error
	if url != "" {
		path, format, err = fetchRemoteToTemp(ctx, o, url)
		if err != nil {
			return ExtractWireResult{}, err
		}
		defer func() { _ = os.Remove(path) }()
	} else {
		path, format, err = localFilePathForExtract(ctx, filePath, o)
		if err != nil {
			return ExtractWireResult{}, err
		}
	}

	format = strings.ToLower(strings.TrimPrefix(format, "."))
	text, err := extractTextByFormat(ctx, path, format, o)
	if err != nil {
		return ExtractWireResult{}, err
	}
	return ExtractWireResult{Text: text}, nil
}

func localFilePathForExtract(ctx context.Context, filePath string, o *options) (string, string, error) {
	path := filepath.Clean(filePath)
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/document: stat"); ie != nil {
		return "", "", ie
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: stat: %w", statErr))
	}
	contentCap := contentByteCap(o.maxBytes)
	if info.Size() > int64(contentCap) {
		return "", "", toolsy.MapToolkitCapError(ctx, "toolkit/document: stat size", contentCap, "file", "")
	}
	return path, formatFromURL(path), nil
}

// wrapParseError preserves ToolErrors (validation) and wraps other parse failures as internal errors.
func wrapParseError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := toolsy.AsToolError(err); ok {
		return err
	}
	return toolsy.NewInternalError(fmt.Errorf("toolkit/document: parse: %w", err))
}

// extractTextByFormat parses the file at path according to format (csv, pdf, docx).
func extractTextByFormat(ctx context.Context, path, format string, o *options) (string, error) {
	byteCap := contentByteCap(o.maxBytes)
	switch format {
	case "csv":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: open csv file: %w", openErr))
		}
		defer func() { _ = f.Close() }()
		text, err := parseCSV(ctx, f, byteCap)
		return text, wrapParseError(err)
	case "pdf":
		text, err := parsePDF(ctx, path, byteCap)
		return text, wrapParseError(err)
	case "docx":
		f, openErr := os.Open(path) // #nosec G703 -- path from args; size checked above
		if openErr != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: open docx file: %w", openErr))
		}
		defer func() { _ = f.Close() }()
		if ie := toolsy.ToolkitContextError(ctx, "toolkit/document: stat docx file"); ie != nil {
			return "", ie
		}
		info, statErr := f.Stat()
		if statErr != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: stat docx file: %w", statErr))
		}
		text, err := parseDOCX(ctx, f, info.Size(), byteCap)
		return text, wrapParseError(err)
	default:
		return "", toolsy.NewValidationError("unsupported format: " + format)
	}
}

// copyRemoteResponseToTemp writes resp.Body to a temp file after status check.
func copyRemoteResponseToTemp(
	ctx context.Context,
	resp *http.Response,
	rawURL string,
	o *options,
) (string, string, error) {
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return "", "", toolsy.NewValidationError("remote file fetch failed: " + resp.Status)
	}
	format := formatFromURL(rawURL)
	if format == "" {
		format = formatFromContentType(resp.Header.Get("Content-Type"))
	}
	contentCap := contentByteCap(o.maxBytes)
	data, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, contentCap)
	if mapped := toolsy.MapToolkitReadError(
		ctx, err, "toolkit/document: read remote body", contentCap, "remote file", "",
	); mapped != nil {
		return "", "", mapped
	}
	if err != nil {
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: read remote body: %w", err))
	}
	tmp, createErr := os.CreateTemp("", "document-*")
	if createErr != nil {
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: create temp: %w", createErr))
	}
	tmpPath := tmp.Name()
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: write temp: %w", writeErr))
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: close temp: %w", closeErr))
	}
	return tmpPath, format, nil
}

// fetchRemoteToTemp downloads a remote document to a temp file and returns path and detected format.
func fetchRemoteToTemp(ctx context.Context, o *options, rawURL string) (string, string, error) {
	u, err := httptool.ValidateRemoteURLWithBlacklist(ctx, rawURL, o.allowPrivateIPs, nil)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: request: %w", err))
	}
	client, err := documentHTTPClient(o)
	if err != nil {
		return "", "", err
	}
	resp, doErr := client.Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if doErr != nil {
		if toolErrorClientCorrectable(doErr) {
			return "", "", doErr
		}
		return "", "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: fetch: %w", doErr))
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	return copyRemoteResponseToTemp(ctx, resp, rawURL, o)
}

func documentHTTPClient(o *options) (*http.Client, error) {
	if o.httpClient != nil {
		if _, ok := o.httpClient.(*http.Client); !ok {
			return nil, errors.New(
				"toolkit/document: default SSRF protection requires *http.Client; pass WithHTTPClient(&http.Client{...})",
			)
		}
	}
	safe := httptool.NewSafeHTTPClient(
		httptool.SafeDialOptions{AllowPrivateIPs: o.allowPrivateIPs}, //nolint:exhaustruct // IP-only blacklist mode
		httptool.CheckRedirectRemote(o.allowPrivateIPs, nil),
	)
	return httptool.MergeHTTPClient(safe, o.httpClient), nil
}

func toolErrorClientCorrectable(err error) bool {
	te, ok := toolsy.AsToolError(err)
	return ok && toolsy.ClientCorrectable(te.Code)
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
