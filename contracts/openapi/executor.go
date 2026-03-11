package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
)

const truncationSuffix = "\n[Truncated. Use pagination or filters.]"

// execute runs the HTTP request for one operation: path params in path, query params in query string, body params in body only.
func execute(ctx context.Context, method, pathTemplate string, pathParamNames, queryParamNames []string, bodyParamNames []string, argsJSON []byte, opts *Options, yield func(toolsy.Chunk) error) error {
	client := opts.httpClient()
	baseURL := strings.TrimSuffix(opts.BaseURL, "/")
	if baseURL == "" {
		return fmt.Errorf("openapi: base URL required (set Options.BaseURL or add servers to the OpenAPI spec)")
	}

	var args map[string]any
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return fmt.Errorf("openapi: invalid args JSON: %w", err)
	}

	pathParamsSet := make(map[string]bool)
	for _, p := range pathParamNames {
		pathParamsSet[p] = true
	}
	queryParamsSet := make(map[string]bool)
	for _, p := range queryParamNames {
		queryParamsSet[p] = true
	}

	path := pathTemplate
	for k, v := range args {
		if !pathParamsSet[k] {
			continue
		}
		placeholder := "{" + k + "}"
		if strings.Contains(path, placeholder) {
			path = strings.ReplaceAll(path, placeholder, url.PathEscape(fmt.Sprint(v)))
		}
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("openapi: base URL: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + strings.TrimPrefix(path, "/")

	q := u.Query()
	for k, v := range args {
		if pathParamsSet[k] || !queryParamsSet[k] {
			continue
		}
		q.Set(k, fmt.Sprint(v))
	}
	u.RawQuery = q.Encode()

	var body io.Reader
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		if len(bodyParamNames) > 0 {
			bodyObj := make(map[string]any)
			for _, k := range bodyParamNames {
				if v, ok := args[k]; ok {
					bodyObj[k] = v
				}
			}
			bodyBytes, err := json.Marshal(bodyObj)
			if err != nil {
				return fmt.Errorf("openapi: marshal body: %w", err)
			}
			body = bytes.NewReader(bodyBytes)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("openapi: request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts.AuthHeader != "" {
		req.Header.Set("Authorization", opts.AuthHeader)
	}

	resp, err := client.Do(req) // #nosec G704 -- URL from Options/spec, not user input
	if err != nil {
		return fmt.Errorf("openapi: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("openapi: read response: %w", err)
	}

	maxBytes := opts.maxResponseBytes()
	if maxBytes > 0 && len(data) > maxBytes {
		truncated := make([]byte, maxBytes, maxBytes+len(truncationSuffix))
		copy(truncated, data[:maxBytes])
		truncated = append(truncated, truncationSuffix...)
		data = truncated
	}

	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: data})
}
