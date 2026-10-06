package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func execute(ctx context.Context, run *toolsy.RunEnv, name, method, path string, contract *operationContract,
	argsJSON []byte, opts *Options, yield func(toolsy.Chunk) error) error {
	decoded, err := jsonschemax.Decode(argsJSON)
	if err != nil {
		return fmt.Errorf("openapi: args: %w", err)
	}
	args := object(decoded)
	if args == nil {
		return errors.New("openapi: args must be an object")
	}
	requestURL, err := makeRequestURL(opts.BaseURL, path, contract, args)
	if err != nil {
		return err
	}
	var body io.Reader
	if value, present := args["body"]; present && contract.body {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return fmt.Errorf("openapi: body: %w", marshalErr)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("openapi: request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if run != nil && run.Credentials != nil {
		auth, authErr := run.Credentials.GetAuth(ctx, name)
		if authErr != nil {
			return fmt.Errorf("openapi: credentials for %s: %w", name, authErr)
		}
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
	}
	// #nosec G704 -- authority comes from host Options or the source contract, never argument input.
	client, owned, clientErr := opts.httpClient()
	if clientErr != nil {
		return clientErr
	}
	if owned {
		defer client.CloseIdleConnections()
	}
	response, err := client.Do(request) //nolint:bodyclose // deferred bounded close helper
	if err != nil {
		return fmt.Errorf("openapi: do request: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, response.Body)
	return emitResponse(ctx, response, contract, opts, yield)
}

func emitResponse(
	ctx context.Context,
	response *http.Response,
	contract *operationContract,
	opts *Options,
	yield func(toolsy.Chunk) error,
) error {
	if !httptool.IsSuccessStatus(response.StatusCode) {
		return fmt.Errorf("openapi: response status %d", response.StatusCode)
	}
	expectJSON, err := declaredResponseJSON(contract, response.StatusCode)
	if err != nil {
		return err
	}

	data, err := textprocessor.ReadLimitedBytes(ctx, response.Body, opts.maxResponseBytes())
	if err != nil {
		return fmt.Errorf("openapi: read response: %w", err)
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	isJSON := mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
	if expectJSON && !isJSON {
		return errors.New("openapi: successful response Content-Type must be JSON")
	}
	if contract.responseJSON != nil && !expectJSON && len(data) > 0 {
		return errors.New("openapi: body returned for an empty response contract")
	}
	if len(data) == 0 {
		if expectJSON {
			return errors.New("openapi: empty body for declared JSON response")
		}
		return yield(toolsy.Chunk{Event: toolsy.EventResult, EmptyResult: true})
	}
	chunkType := toolsy.MimeTypeText
	if isJSON {
		if err := validateJSONResponse(data); err != nil {
			return err
		}
		chunkType = toolsy.MimeTypeJSON
	}

	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: data, MimeType: chunkType})
}

func scalarString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	default:
		return "", errors.New("openapi: parameter must be scalar")
	}
}
func makeRequestURL(base, path string, contract *operationContract, args map[string]any) (string, error) {
	if base == "" {
		return "", errors.New("openapi: base URL required (set Options.BaseURL or add servers to the OpenAPI spec)")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("openapi: base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.Fragment != "" {
		return "", errors.New("openapi: base URL must be an absolute HTTP(S) URL without fragment")
	}
	parts := []string{}
	if parsed.RawQuery != "" {
		parts = append(parts, parsed.RawQuery)
	}
	for _, parameter := range contract.parameters {
		values := object(args[parameter.location])
		value, present := values[parameter.name]
		if !present {
			continue
		}
		if parameter.location == locationPath {
			text, scalarErr := scalarString(value)
			if scalarErr != nil {
				return "", scalarErr
			}
			path = strings.ReplaceAll(path, "{"+parameter.name+"}", escapeParameter(text))
			continue
		}
		serialized, serializationErr := queryParameter(parameter, value)
		if serializationErr != nil {
			return "", serializationErr
		}
		parts = append(parts, serialized...)
	}
	// Escaped parameter values remain in RawPath, so '/' in a value cannot become a path segment.
	escapedPath := strings.TrimSuffix(parsed.EscapedPath(), "/") + "/" + strings.TrimPrefix(path, "/")
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", fmt.Errorf("openapi: path encoding: %w", err)
	}
	parsed.Path, parsed.RawPath = decodedPath, escapedPath
	parsed.RawQuery = strings.Join(parts, "&")
	return parsed.String(), nil
}

func queryParameter(parameter parameter, value any) ([]string, error) {
	parts := []string{}
	escapedName := escapeParameter(parameter.name)
	if !parameter.array {
		text, scalarErr := scalarString(value)
		if scalarErr != nil {
			return nil, scalarErr
		}
		parts = append(parts, escapedName+"="+escapeParameter(text))
		return parts, nil
	}
	array, ok := value.([]any)
	if !ok {
		return nil, errors.New("openapi: query array value expected")
	}
	if len(array) == 0 {
		return nil, nil
	}
	escaped := make([]string, 0, len(array))
	for _, entry := range array {
		text, scalarErr := scalarString(entry)
		if scalarErr != nil {
			return nil, scalarErr
		}
		escaped = append(escaped, escapeParameter(text))
	}
	if parameter.explode {
		for _, entry := range escaped {
			parts = append(parts, escapedName+"="+entry)
		}
	} else {
		parts = append(parts, escapedName+"="+strings.Join(escaped, ","))
	}
	return parts, nil
}

// RFC6570 simple/form expansions encode every byte except RFC3986 unreserved characters.
func escapeParameter(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func validateJSONResponse(data []byte) error {
	if _, err := jsonschemax.Decode(data); err != nil {
		return fmt.Errorf("openapi: invalid JSON response: %w", err)
	}
	return nil
}

func declaredResponseJSON(contract *operationContract, statusCode int) (bool, error) {
	if contract.responseJSON == nil {
		return false, nil
	}
	for _, status := range []string{strconv.Itoa(statusCode), "2XX", defaultKeyword} {
		if value, ok := contract.responseJSON[status]; ok {
			return value, nil
		}
	}
	return false, errors.New("openapi: undeclared successful response status")
}
