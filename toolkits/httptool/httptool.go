package httptool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/textutil"
)

const truncationSuffix = "\n[Truncated]"

type getArgs struct {
	URL string `json:"url"`
}

type httpResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

type postArgs struct {
	URL      string         `json:"url"`
	JSONBody map[string]any `json:"json_body,omitempty"`
}

// AsTools returns two tools: http_get and http_post. Options configure client, allowed domains, headers, and limits.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	if hasForbiddenHeaders(o.headers) {
		return nil, errors.New(
			"toolkit/httptool: static Authorization headers are not allowed; use toolsy.CredentialsProvider",
		)
	}

	getTool, err := toolsy.NewToolWithRun[getArgs, httpResult](
		o.getName,
		o.getDesc,
		func(ctx context.Context, run toolsy.RunContext, args getArgs) (httpResult, error) {
			return doGET(ctx, run, o.getName, &o, args.URL)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/httptool: build get tool: %w", err)
	}

	postTool, err := toolsy.NewToolWithRun[postArgs, httpResult](
		o.postName,
		o.postDesc,
		func(ctx context.Context, run toolsy.RunContext, args postArgs) (httpResult, error) {
			return doPOST(ctx, run, o.postName, &o, args.URL, args.JSONBody)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/httptool: build post tool: %w", err)
	}

	return []toolsy.Tool{getTool, postTool}, nil
}

func doGET(ctx context.Context, run toolsy.RunContext, toolName string, o *options, rawURL string) (httpResult, error) {
	u, err := validateURL(ctx, rawURL, o.allowedDomains, o.allowPrivateIPs)
	if err != nil {
		return httpResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return httpResult{}, fmt.Errorf("toolkit/httptool: new request: %w", err)
	}
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	if run.Credentials != nil {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return httpResult{}, fmt.Errorf("toolkit/httptool: credentials for %s: %w", toolName, authErr)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// G704: URL is validated by validateURL (allowedDomains + private IP check) before Do.
	resp, err := o.httpClient.Do(req) // #nosec G704
	if err != nil {
		return httpResult{}, fmt.Errorf("toolkit/httptool: do request: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	body, err := readAndTruncate(resp.Body, o.maxResponseBody)
	if err != nil {
		return httpResult{}, err
	}
	return httpResult{Status: resp.StatusCode, Body: body}, nil
}

func doPOST(
	ctx context.Context,
	run toolsy.RunContext,
	toolName string,
	o *options,
	rawURL string,
	jsonBody map[string]any,
) (httpResult, error) {
	u, err := validateURL(ctx, rawURL, o.allowedDomains, o.allowPrivateIPs)
	if err != nil {
		return httpResult{}, err
	}

	var reqBody io.Reader
	if len(jsonBody) > 0 {
		bodyBytes, marshalErr := json.Marshal(jsonBody)
		if marshalErr != nil {
			return httpResult{}, fmt.Errorf("toolkit/httptool: marshal body: %w", marshalErr)
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), reqBody)
	if err != nil {
		return httpResult{}, fmt.Errorf("toolkit/httptool: new request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	if run.Credentials != nil {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return httpResult{}, fmt.Errorf("toolkit/httptool: credentials for %s: %w", toolName, authErr)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// G704: URL is validated by validateURL (allowedDomains + private IP check) before Do.
	resp, err := o.httpClient.Do(req) // #nosec G704
	if err != nil {
		return httpResult{}, fmt.Errorf("toolkit/httptool: do request: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	body, err := readAndTruncate(resp.Body, o.maxResponseBody)
	if err != nil {
		return httpResult{}, err
	}
	return httpResult{Status: resp.StatusCode, Body: body}, nil
}

// readAndTruncate reads up to maxBytes from r. If more than maxBytes are available, returns
// UTF-8 safe truncation plus truncationSuffix. Caller must drain r after return (e.g. via defer).
func readAndTruncate(r io.Reader, maxBytes int) (string, error) {
	text, err := textutil.ReadAndTruncateValidUTF8(r, maxBytes, truncationSuffix)
	if err != nil {
		return "", fmt.Errorf("toolkit/httptool: read body: %w", err)
	}
	return text, nil
}
