package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/skosovsky/toolsy/internal/jsonschemax"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// ParseURL fetches the OpenAPI spec from specURL, parses it, filters by opts, and returns one toolsy.Tool per operation.
func ParseURL(ctx context.Context, specURL string, opts Options) ([]toolsy.Tool, error) {
	for _, method := range opts.AllowedMethods {
		switch method {
		case http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions:
		default:
			return nil, unsupported("selected HTTP method " + method)
		}
	}
	client := opts.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, fmt.Errorf("openapi: request: %w", err)
	}
	resp, err := client.Do(req) //nolint:bodyclose // drained and closed via httptool.CloseResponseBody
	if err != nil {
		return nil, fmt.Errorf("openapi: fetch spec: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("openapi: spec status %d", resp.StatusCode)
	}
	data, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, defaultMaxSpecBytes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if toolsy.IsContextInterrupt(err) {
			return nil, err
		}
		if textprocessor.IsReadLimitExceeded(err) {
			return nil, fmt.Errorf("openapi: spec exceeds %d byte limit: %w", defaultMaxSpecBytes, err)
		}
		return nil, fmt.Errorf("openapi: read spec: %w", err)
	}

	decoded, err := jsonschemax.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("openapi: JSON spec: %w", err)
	}
	source, ok := decoded.(map[string]any)
	if !ok {
		return nil, unsupported("document must be an object")
	}
	version, _ := source["openapi"].(string)
	if !strings.HasPrefix(version, "3.0.") {
		return nil, unsupported("only JSON OpenAPI 3.0.x is supported")
	}
	if referenceErr := checkReferences(source); referenceErr != nil {
		return nil, referenceErr
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("openapi: parse spec: %w", err)
	}

	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("openapi: invalid contract: %w", err)
	}
	return docToTools(doc, source, specURL, &opts)
}

// docToTools constructs tools only after the entire selected set has passed construction.
func docToTools(doc *openapi3.T, source map[string]any, specURL string, opts *Options) ([]toolsy.Tool, error) {
	paths := object(source["paths"])
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	used := make(map[string]bool)
	var tools []toolsy.Tool
	for _, path := range ordered {
		item := doc.Paths.Value(path)
		if item == nil {
			continue
		}
		rawItem := object(paths[path])
		if _, reference := rawItem["$ref"]; reference {
			return nil, unsupported("path-item references")
		}
		if includeOperation(item.Trace, http.MethodTrace, opts) {
			return nil, unsupported("TRACE operation " + path)
		}
		for _, method := range []string{http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut} {
			operation := item.GetOperation(method)
			if !includeOperation(operation, method, opts) {
				continue
			}
			tool, err := operationTool(operation, source, rawItem, specURL, method, path, used, opts)
			if err != nil {
				return nil, err
			}

			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func operationBaseURL(source, item, operation map[string]any, specURL, override string) (string, error) {
	base := override
	if base == "" {
		var resolutionErr error
		base, resolutionErr = sourceBaseURL(source, item, operation, specURL)
		if resolutionErr != nil {
			return "", resolutionErr
		}
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("openapi: server URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Fragment != "" ||
		parsed.User != nil {
		return "", unsupported("server must resolve to HTTP(S) without fragment or userinfo")
	}
	return base, nil
}

func operationTool(
	operation *openapi3.Operation,
	source, rawItem map[string]any,
	specURL, method, path string,
	used map[string]bool,
	opts *Options,
) (toolsy.Tool, error) {
	rawOperation := object(rawItem[strings.ToLower(method)])
	working := *opts
	base, err := operationBaseURL(source, rawItem, rawOperation, specURL, opts.BaseURL)
	if err != nil {
		return nil, err
	}
	working.BaseURL = base
	contract, err := buildOperationContract(source, rawItem, rawOperation, method, path)
	if err != nil {
		return nil, fmt.Errorf("openapi: %s %s: %w", method, path, err)
	}
	name := toolNameFromOperation(operation.OperationID, strings.ToLower(method), path, used)
	description := operation.Summary
	if description == "" {
		description = operation.Description
	}
	if description == "" {
		description = method + " " + path
	}
	encoded, err := json.Marshal(contract.input)
	if err != nil {
		return nil, fmt.Errorf("openapi: schema: %w", err)
	}
	toolOptions := []toolsy.ToolOption{}
	if contract.output != nil {
		toolOptions = append(toolOptions, toolsy.WithOutputSchema(contract.output))
	}
	tool, err := toolsy.NewProxyTool(name, description, encoded,
		func(ctx context.Context, run *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			return execute(ctx, run, name, method, path, contract, args, &working, yield)
		}, toolOptions...)
	if err != nil {
		return nil, fmt.Errorf("openapi: tool %s: %w", name, err)
	}
	return tool, nil
}

func sourceBaseURL(source, item, operation map[string]any, specURL string) (string, error) {
	var base string

	base = "/"
	servers := source["servers"]
	if value, ok := item["servers"]; ok {
		servers = value
	}
	if value, ok := operation["servers"]; ok {
		servers = value
	}
	if list, ok := servers.([]any); ok && len(list) > 0 {
		server := object(list[0])
		base = stringValue(server["url"])
		variables := object(server["variables"])
		for _, name := range keys(variables) {
			base = strings.ReplaceAll(base, "{"+name+"}", stringValue(object(variables[name])["default"]))
		}
	}
	origin, err := url.Parse(specURL)
	if err != nil {
		return "", fmt.Errorf("openapi: spec URL: %w", err)
	}
	relative, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("openapi: server URL: %w", err)
	}
	base = origin.ResolveReference(relative).String()
	return base, nil
}
