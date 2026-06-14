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
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)
	o.httpClient = defaultHTTPClient(&o)
	if hasForbiddenHeaders(o.headers) {
		return nil, errors.New(
			"toolkit/httptool: static Authorization headers are not allowed; use toolsy.CredentialsProvider",
		)
	}

	getTool, err := toolsy.NewTool[getArgs, httpResult](
		o.getName,
		o.getDesc,
		func(ctx context.Context, run *toolsy.RunEnv, args getArgs) (httpResult, error) {
			return doGET(ctx, run, o.getName, &o, args.URL)
		},
		toolsy.WithReadOnly(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/httptool: build get tool: %w", err)
	}

	postTool, err := toolsy.NewTool[postArgs, httpResult](
		o.postName,
		o.postDesc,
		func(ctx context.Context, run *toolsy.RunEnv, args postArgs) (httpResult, error) {
			return doPOST(ctx, run, o.postName, &o, args.URL, args.JSONBody)
		},
		toolsy.WithDangerous(),
		toolsy.WithRequiresConfirmation(),
	)
	if err != nil {
		return nil, fmt.Errorf("toolkit/httptool: build post tool: %w", err)
	}

	return []toolsy.Tool{getTool, postTool}, nil
}

func doGET(ctx context.Context, run *toolsy.RunEnv, toolName string, o *options, rawURL string) (httpResult, error) {
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
	if run.Credentials != nil {
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
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: do request: %w", err))
	}
	defer CloseResponseBody(ctx, resp.Body)

	body, err := ReadBodyLimited(ctx, resp.Body, o.maxResponseBody)
	if err != nil {
		return httpResult{}, err
	}
	return httpResult{Status: resp.StatusCode, Body: body}, nil
}

func doPOST(
	ctx context.Context,
	run *toolsy.RunEnv,
	toolName string,
	o *options,
	rawURL string,
	jsonBody json.RawMessage,
) (httpResult, error) {
	u, err := validateURL(ctx, rawURL, o.allowedDomains, o.allowPrivateIPs)
	if err != nil {
		return httpResult{}, err
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
	if run.Credentials != nil {
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
		return httpResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: do request: %w", err))
	}
	defer CloseResponseBody(ctx, resp.Body)

	body, err := ReadBodyLimited(ctx, resp.Body, o.maxResponseBody)
	if err != nil {
		return httpResult{}, err
	}
	return httpResult{Status: resp.StatusCode, Body: body}, nil
}
