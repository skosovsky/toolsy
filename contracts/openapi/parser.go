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
	tools, _, err := ParseURLWithCleanup(ctx, specURL, opts)
	return tools, err
}

// ParseURLWithCleanup returns tools sharing one owned safe pool and its idle closer.
// Stop new calls before disposal; active calls remain unaffected.
func ParseURLWithCleanup(ctx context.Context, specURL string, opts Options) ([]toolsy.Tool, func(), error) {
	client, _, clientErr := opts.httpClient()
	if clientErr != nil {
		return nil, nil, clientErr
	}
	opts.client = client
	success := false
	defer func() {
		if !success {
			client.CloseIdleConnections()
		}
	}()
	if methodErr := validateSelectedMethods(opts.AllowedMethods); methodErr != nil {
		return nil, nil, methodErr
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("openapi: request: %w", err)
	}
	resp, err := client.Do(req) //nolint:bodyclose // drained and closed via httptool.CloseResponseBody
	if err != nil {
		return nil, nil, fmt.Errorf("openapi: fetch spec: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return nil, nil, fmt.Errorf("openapi: spec status %d", resp.StatusCode)
	}
	data, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, defaultMaxSpecBytes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if toolsy.IsContextInterrupt(err) {
			return nil, nil, err
		}
		if textprocessor.IsReadLimitExceeded(err) {
			return nil, nil, fmt.Errorf("openapi: spec exceeds %d byte limit: %w", defaultMaxSpecBytes, err)
		}
		return nil, nil, fmt.Errorf("openapi: read spec: %w", err)
	}

	doc, source, docErr := loadOpenAPIDocument(ctx, data)
	if docErr != nil {
		return nil, nil, docErr
	}
	tools, buildErr := docToTools(doc, source, specURL, &opts)
	if buildErr != nil {
		return nil, nil, buildErr
	}
	success = true
	return tools, client.CloseIdleConnections, nil
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

func validateSelectedMethods(methods []string) error {
	for _, method := range methods {
		switch method {
		case http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions:
		default:
			return unsupported("selected HTTP method " + method)
		}
	}
	return nil
}

func loadOpenAPIDocument(ctx context.Context, data []byte) (*openapi3.T, map[string]any, error) {
	decoded, err := jsonschemax.Decode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("openapi: JSON spec: %w", err)
	}
	source, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil, unsupported("document must be an object")
	}
	version, _ := source["openapi"].(string)
	if !strings.HasPrefix(version, "3.0.") {
		return nil, nil, unsupported("only JSON OpenAPI 3.0.x is supported")
	}
	if referenceErr := checkReferences(source); referenceErr != nil {
		return nil, nil, referenceErr
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, nil, fmt.Errorf("openapi: parse spec: %w", err)
	}

	if err := doc.Validate(ctx); err != nil {
		return nil, nil, fmt.Errorf("openapi: invalid contract: %w", err)
	}
	return doc, source, nil
}
