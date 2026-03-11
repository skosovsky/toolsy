package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/skosovsky/toolsy"
)

// introspectionQuery uses fragment TypeRef for full type depth (e.g. [String!] -> NON_NULL(LIST(NON_NULL(SCALAR)))).
const introspectionQuery = `query IntrospectionQuery { __schema { queryType { name } mutationType { name } types { name kind fields { name args { name type { ...TypeRef } } } } } } fragment TypeRef on __Type { name kind ofType { ...TypeRef } }`

type introResponse struct {
	Data   *introData   `json:"data"`
	Errors []introError `json:"errors,omitempty"`
}

type introData struct {
	Schema struct {
		QueryType    *struct{ Name string } `json:"queryType"`
		MutationType *struct{ Name string } `json:"mutationType"`
		Types        []struct {
			Name   string       `json:"name"`
			Kind   string       `json:"kind"`
			Fields []introField `json:"fields"`
		} `json:"types"`
	} `json:"__schema"`
}

type introError struct {
	Message string `json:"message"`
}

type introField struct {
	Name string    `json:"name"`
	Args []ArgSpec `json:"args"`
}

// Introspect calls the GraphQL endpoint with the introspection query, then builds one tool per root query/mutation.
func Introspect(ctx context.Context, endpoint string, opts Options) ([]toolsy.Tool, error) {
	client := opts.httpClient()
	body := map[string]string{"query": introspectionQuery}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("graphql: marshal intro body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("graphql: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.AuthHeader != "" {
		req.Header.Set("Authorization", opts.AuthHeader)
	}
	resp, err := client.Do(req) // #nosec G704 -- endpoint from caller config, not user input
	if err != nil {
		return nil, fmt.Errorf("graphql: introspect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("graphql: read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("graphql: introspection HTTP %d: %s", resp.StatusCode, string(data))
	}
	var ir introResponse
	if err := json.Unmarshal(data, &ir); err != nil {
		return nil, fmt.Errorf("graphql: parse intro: %w", err)
	}
	if len(ir.Errors) > 0 {
		msg := ir.Errors[0].Message
		if msg == "" {
			msg = "introspection returned errors"
		}
		return nil, fmt.Errorf("graphql: introspection errors: %s", msg)
	}
	if ir.Data == nil {
		return nil, fmt.Errorf("graphql: introspection: no data in response")
	}
	schema := ir.Data.Schema
	allowedOps := opts.Operations
	if len(allowedOps) == 0 {
		allowedOps = []string{"query", "mutation"}
	}
	allowedSet := make(map[string]bool)
	for _, o := range allowedOps {
		allowedSet[strings.ToLower(o)] = true
	}

	typeMap := make(map[string][]introField)
	for _, t := range schema.Types {
		typeMap[t.Name] = t.Fields
	}

	var tools []toolsy.Tool
	usedNames := make(map[string]bool)

	if allowedSet["query"] && schema.QueryType != nil {
		rootName := schema.QueryType.Name
		if fields, ok := typeMap[rootName]; ok {
			for _, f := range fields {
				name := toolName(f.Name, usedNames)
				schemaBytes, err := argsToJSONSchema(f.Args)
				if err != nil {
					return nil, fmt.Errorf("graphql: schema %s: %w", f.Name, err)
				}
				queryText := buildStaticQuery("query", f.Name, f.Args)
				endpointCopy := endpoint
				optsCopy := opts
				tool, err := toolsy.NewProxyTool(name, "GraphQL query: "+f.Name, schemaBytes, func(ctx context.Context, argsJSON []byte, yield func(toolsy.Chunk) error) error {
					return executeGraphQL(ctx, endpointCopy, queryText, argsJSON, &optsCopy, yield)
				})
				if err != nil {
					return nil, fmt.Errorf("graphql: tool %s: %w", name, err)
				}
				tools = append(tools, tool)
			}
		}
	}
	if allowedSet["mutation"] && schema.MutationType != nil {
		rootName := schema.MutationType.Name
		if fields, ok := typeMap[rootName]; ok {
			for _, f := range fields {
				name := toolName(f.Name, usedNames)
				schemaBytes, err := argsToJSONSchema(f.Args)
				if err != nil {
					return nil, fmt.Errorf("graphql: schema %s: %w", f.Name, err)
				}
				queryText := buildStaticQuery("mutation", f.Name, f.Args)
				endpointCopy := endpoint
				optsCopy := opts
				tool, err := toolsy.NewProxyTool(name, "GraphQL mutation: "+f.Name, schemaBytes, func(ctx context.Context, argsJSON []byte, yield func(toolsy.Chunk) error) error {
					return executeGraphQL(ctx, endpointCopy, queryText, argsJSON, &optsCopy, yield)
				})
				if err != nil {
					return nil, fmt.Errorf("graphql: tool %s: %w", name, err)
				}
				tools = append(tools, tool)
			}
		}
	}
	return tools, nil
}

func executeGraphQL(ctx context.Context, endpoint, queryText string, argsJSON []byte, opts *Options, yield func(toolsy.Chunk) error) error {
	var variables map[string]any
	if len(argsJSON) > 0 {
		if err := json.Unmarshal(argsJSON, &variables); err != nil {
			return fmt.Errorf("graphql: variables: %w", err)
		}
	}
	if variables == nil {
		variables = make(map[string]any)
	}
	body := map[string]any{"query": queryText, "variables": variables}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("graphql: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("graphql: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.AuthHeader != "" {
		req.Header.Set("Authorization", opts.AuthHeader)
	}
	resp, err := opts.httpClient().Do(req) // #nosec G704 -- endpoint from caller config, not user input
	if err != nil {
		return fmt.Errorf("graphql: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("graphql: read: %w", err)
	}
	const truncationSuffix = "\n[Truncated. Use pagination or filters.]"
	maxBytes := opts.maxResponseBytes()
	if maxBytes > 0 && len(data) > maxBytes {
		truncated := make([]byte, maxBytes, maxBytes+len(truncationSuffix))
		copy(truncated, data[:maxBytes])
		truncated = append(truncated, truncationSuffix...)
		data = truncated
	}
	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: data})
}
