package httptool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/skosovsky/toolsy/internal/format"

	"github.com/skosovsky/toolsy"
)

type getArgs struct {
	URL string `json:"url"`
}

type httpResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

type postArgs struct {
	URL      string          `json:"url"`
	JSONBody json.RawMessage `json:"json_body,omitempty"`
}

// AsTools returns two tools: http_get and http_post. Options configure client, allowed domains, headers, and limits.
func AsTools(opts ...Option) ([]toolsy.Tool, error) {
	value, _, err := AsToolsWithCleanup(opts...)
	return value, err
}

// AsToolsWithCleanup returns the tools and an owned idle-pool closer. Stop new calls
// before disposal; the closer leaves active calls unaffected and is not terminal Close.
func AsToolsWithCleanup(opts ...Option) ([]toolsy.Tool, func(), error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	if err := normalizeCredentialOrigins(&o); err != nil {
		return nil, nil, err
	}
	client, clientErr := defaultHTTPClient(&o)
	if clientErr != nil {
		return nil, nil, clientErr
	}
	o.httpClient = client
	success := false
	defer func() {
		if !success {
			client.CloseIdleConnections()
		}
	}()
	if hasForbiddenHeaders(o.headers) {
		return nil, nil, errors.New(
			"toolkit/httptool: static authentication headers are not allowed; use explicit credential origin binding",
		)
	}

	getTool, err := toolsy.NewTool[getArgs, format.JSONResult](
		o.getName,
		o.getDesc,
		func(ctx context.Context, run *toolsy.RunEnv, args getArgs) (format.JSONResult, error) {
			r, requestErr := doGET(ctx, run, o.getName, &o, args.URL)
			if requestErr != nil {
				return format.JSONResult{}, requestErr
			}
			return format.ToJSONResult(r, o.maxWireBytes)
		},
		toolsy.WithReadOnly(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("toolkit/httptool: build get tool: %w", err)
	}

	postTool, err := toolsy.NewTool[postArgs, format.JSONResult](
		o.postName,
		o.postDesc,
		func(ctx context.Context, run *toolsy.RunEnv, args postArgs) (format.JSONResult, error) {
			r, requestErr := doPOST(ctx, run, o.postName, &o, args.URL, args.JSONBody)
			if requestErr != nil {
				return format.JSONResult{}, requestErr
			}
			return format.ToJSONResult(r, o.maxWireBytes)
		},
		toolsy.WithDangerous(),
		toolsy.WithRequiresConfirmation(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("toolkit/httptool: build post tool: %w", err)
	}

	success = true
	return []toolsy.Tool{getTool, postTool}, client.CloseIdleConnections, nil
}

func doGET(ctx context.Context, run *toolsy.RunEnv, toolName string, o *options, rawURL string) (httpResult, error) {
	if len(rawURL) > maxURLBytes {
		return httpResult{}, toolsy.NewValidationError("URL exceeds 8192 bytes")
	}
	u, err := validateURL(ctx, rawURL, o.allowedDomains, o.allowPrivateIPs)
	if err != nil {
		return httpResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: new request: %w", err))
	}
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	if credentialsAllowed(run, o, u) {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return httpResult{}, toolsy.NewInternalError(
				fmt.Errorf("toolkit/httptool: credentials for %s: %w", toolName, authErr),
			)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// G704: URL is validated by validateURL (allowedDomains + private IP check) before Do.
	resp, err := o.httpClient.Do(req) //nolint:bodyclose // closed via CloseResponseBody
	if err != nil {
		if _, ok := toolsy.AsToolError(err); ok {
			return httpResult{}, err
		}
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: do request: %w", err))
	}
	defer CloseResponseBody(ctx, resp.Body)

	body, err := ReadBodyLimited(ctx, resp.Body, o.maxResponseBody)
	if mapped := toolsy.MapToolkitReadError(
		ctx, err, "toolkit/httptool: read body", o.maxResponseBody, "response body", "",
	); mapped != nil {
		return httpResult{}, mapped
	}
	if err != nil {
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: read body: %w", err))
	}
	return httpResult{Status: resp.StatusCode, Body: string(body)}, nil
}

func doPOST(
	ctx context.Context,
	run *toolsy.RunEnv,
	toolName string,
	o *options,
	rawURL string,
	jsonBody json.RawMessage,
) (httpResult, error) {
	if len(rawURL) > maxURLBytes {
		return httpResult{}, toolsy.NewValidationError("URL exceeds 8192 bytes")
	}
	u, err := validateURL(ctx, rawURL, o.allowedDomains, o.allowPrivateIPs)
	if err != nil {
		return httpResult{}, err
	}

	if len(jsonBody) > o.maxRequestBody {
		return httpResult{}, toolsy.NewValidationError("request body exceeds configured byte limit")
	}
	var reqBody io.Reader
	if len(jsonBody) > 0 {
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), reqBody)
	if err != nil {
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: new request: %w", err))
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	if credentialsAllowed(run, o, u) {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return httpResult{}, toolsy.NewInternalError(
				fmt.Errorf("toolkit/httptool: credentials for %s: %w", toolName, authErr),
			)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// G704: URL is validated by validateURL (allowedDomains + private IP check) before Do.
	resp, err := o.httpClient.Do(req) //nolint:bodyclose // closed via CloseResponseBody
	if err != nil {
		if _, ok := toolsy.AsToolError(err); ok {
			return httpResult{}, err
		}
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: do request: %w", err))
	}
	defer CloseResponseBody(ctx, resp.Body)

	body, err := ReadBodyLimited(ctx, resp.Body, o.maxResponseBody)
	if mapped := toolsy.MapToolkitReadError(
		ctx, err, "toolkit/httptool: read body", o.maxResponseBody, "response body", "",
	); mapped != nil {
		return httpResult{}, mapped
	}
	if err != nil {
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: read body: %w", err))
	}
	return httpResult{Status: resp.StatusCode, Body: string(body)}, nil
}

func credentialsAllowed(run *toolsy.RunEnv, o *options, u *url.URL) bool {
	return run != nil && run.Credentials != nil && slices.Contains(o.credentialOrigins, origin(u))
}
