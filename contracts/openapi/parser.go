package openapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/skosovsky/toolsy"
)

// ParseURL fetches the OpenAPI spec from specURL, parses it, filters by opts, and returns one toolsy.Tool per operation.
func ParseURL(ctx context.Context, specURL string, opts Options) ([]toolsy.Tool, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, fmt.Errorf("openapi: request: %w", err)
	}
	resp, err := opts.HTTPClient.Do(req) // #nosec G704 -- specURL from caller config, not user input
	if err != nil {
		return nil, fmt.Errorf("openapi: fetch spec: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openapi: spec status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openapi: read spec: %w", err)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("openapi: parse spec: %w", err)
	}

	return docToTools(ctx, doc, &opts)
}

// serverURLWithDefaults returns server URL with {variable} placeholders replaced by Server.Variables[].Default when set.
func serverURLWithDefaults(s openapi3.Server) string {
	u := s.URL
	if u == "" {
		return ""
	}
	if s.Variables == nil {
		return u
	}
	for name, v := range s.Variables {
		if v != nil && v.Default != "" {
			u = strings.ReplaceAll(u, "{"+name+"}", v.Default)
		}
	}
	return u
}

func docToTools(_ context.Context, doc *openapi3.T, opts *Options) ([]toolsy.Tool, error) {
	workingOpts := *opts
	if workingOpts.BaseURL == "" && len(doc.Servers) > 0 {
		if url := serverURLWithDefaults(*doc.Servers[0]); url != "" {
			workingOpts.BaseURL = url
		}
	}
	opts = &workingOpts

	var tools []toolsy.Tool
	usedNames := make(map[string]bool)

	for path, pathItem := range doc.Paths.Map() {
		if pathItem == nil {
			continue
		}
		forPath, err := toolsForPath(path, pathItem, opts, usedNames)
		if err != nil {
			return nil, err
		}
		tools = append(tools, forPath...)
	}
	return tools, nil
}

func toolsForPath(
	path string,
	pathItem *openapi3.PathItem,
	opts *Options,
	usedNames map[string]bool,
) ([]toolsy.Tool, error) {
	var tools []toolsy.Tool
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		op := operationForMethod(pathItem, method)
		if !includeOperation(op, method, opts) {
			continue
		}
		name := toolNameFromOperation(op.OperationID, strings.ToLower(method), path, usedNames)
		desc := op.Summary
		if desc == "" {
			desc = op.Description
		}
		if desc == "" {
			desc = method + " " + path
		}
		schemaBytes, err := operationToJSONSchema(op, pathItem)
		if err != nil {
			return nil, fmt.Errorf("openapi: schema %s %s: %w", method, path, err)
		}
		pathNames, queryNames, bodyNames := operationParamSets(op, pathItem, path)
		pathTemplate := path
		methodCopy := method
		optsCopy := *opts
		tool, err := toolsy.NewProxyTool(
			name,
			desc,
			schemaBytes,
			func(ctx context.Context, argsJSON []byte, yield func(toolsy.Chunk) error) error {
				return execute(
					ctx,
					methodCopy,
					pathTemplate,
					pathNames,
					queryNames,
					bodyNames,
					argsJSON,
					&optsCopy,
					yield,
				)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("openapi: tool %s: %w", name, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func operationForMethod(pathItem *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return pathItem.Get
	case http.MethodPost:
		return pathItem.Post
	case http.MethodPut:
		return pathItem.Put
	case http.MethodPatch:
		return pathItem.Patch
	case http.MethodDelete:
		return pathItem.Delete
	default:
		return nil
	}
}
