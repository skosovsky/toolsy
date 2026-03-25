package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/textutil"
)

const truncationSuffix = "\n[Truncated. Use pagination or filters.]"

// execute runs the HTTP request for one operation: path params in path, query params in query string, body params in body only.
func execute(
	ctx context.Context,
	run toolsy.RunContext,
	toolName string,
	method, pathTemplate string,
	pathParamNames, queryParamNames []string,
	bodyParamNames []string,
	argsJSON []byte,
	opts *Options,
	yield func(toolsy.Chunk) error,
) error {
	client := opts.httpClient()
	baseURL := strings.TrimSuffix(opts.BaseURL, "/")
	if baseURL == "" {
		return errors.New("openapi: base URL required (set Options.BaseURL or add servers to the OpenAPI spec)")
	}

	args, err := parseArgsJSON(argsJSON)
	if err != nil {
		return err
	}

	pathParamsSet := paramNameSet(pathParamNames)
	queryParamsSet := paramNameSet(queryParamNames)

	substitutedPath := substitutePathParams(pathTemplate, args, pathParamsSet)
	u, err := buildRequestURL(baseURL, substitutedPath, args, pathParamsSet, queryParamsSet)
	if err != nil {
		return err
	}

	var body io.Reader
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		b, marshalErr := marshalRequestBodyJSON(bodyParamNames, args)
		if marshalErr != nil {
			return marshalErr
		}
		if len(bodyParamNames) > 0 {
			body = bytes.NewReader(b)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("openapi: request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if run.Credentials != nil {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return fmt.Errorf("openapi: credentials for %s: %w", toolName, authErr)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}

	resp, err := client.Do(req) // #nosec G704 -- URL from Options/spec, not user input
	if err != nil {
		return fmt.Errorf("openapi: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	text, err := textutil.ReadAndTruncateValidUTF8(resp.Body, opts.maxResponseBytes(), truncationSuffix)
	if err != nil {
		return fmt.Errorf("openapi: read response: %w", err)
	}

	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(text), MimeType: toolsy.MimeTypeText})
}

func parseArgsJSON(argsJSON []byte) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return nil, fmt.Errorf("openapi: invalid args JSON: %w", err)
	}
	return args, nil
}

func paramNameSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, p := range names {
		m[p] = true
	}
	return m
}

func substitutePathParams(pathTemplate string, args map[string]any, pathParamsSet map[string]bool) string {
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
	return path
}

func buildRequestURL(
	baseURL, substitutedPath string,
	args map[string]any,
	pathParamsSet, queryParamsSet map[string]bool,
) (*url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("openapi: base URL: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + strings.TrimPrefix(substitutedPath, "/")

	q := u.Query()
	for k, v := range args {
		if pathParamsSet[k] || !queryParamsSet[k] {
			continue
		}
		q.Set(k, fmt.Sprint(v))
	}
	u.RawQuery = q.Encode()
	return u, nil
}

func marshalRequestBodyJSON(bodyParamNames []string, args map[string]any) ([]byte, error) {
	if len(bodyParamNames) == 0 {
		return nil, nil
	}
	bodyObj := make(map[string]any)
	for _, k := range bodyParamNames {
		if v, ok := args[k]; ok {
			bodyObj[k] = v
		}
	}
	bodyBytes, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, fmt.Errorf("openapi: marshal body: %w", err)
	}
	return bodyBytes, nil
}
