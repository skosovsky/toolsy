package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	operationQuery = "query"
)

// Type references are finite; incomplete references are rejected by the mapper.
const introspectionQuery = `query IntrospectionQuery { __schema { queryType { name } mutationType { name } types { name kind isOneOf enumValues(includeDeprecated: true) { name } inputFields(includeDeprecated: true) { name defaultValue type { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind } } } } } } } } } } } } } } } } } } fields(includeDeprecated: true) { name type { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind } } } } } } } } } } } } } } } } } args(includeDeprecated: true) { name defaultValue type { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind ofType { name kind } } } } } } } } } } } } } } } } } } } } } }`

type introResponse struct {
	Data   *introData   `json:"data"`
	Errors []introError `json:"errors,omitempty"`
}

type introTypeName struct {
	Name string `json:"name"`
}

type introSchemaType struct {
	IsOneOf     bool            `json:"isOneOf"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Fields      []introField    `json:"fields"`
	InputFields []ArgSpec       `json:"inputFields"`
	EnumValues  []introTypeName `json:"enumValues"`
}

type introSchema struct {
	QueryType    *introTypeName    `json:"queryType"`
	MutationType *introTypeName    `json:"mutationType"`
	Types        []introSchemaType `json:"types"`
}

type introData struct {
	Schema introSchema `json:"__schema"`
}

type introError struct {
	Message string `json:"message"`
}

type introField struct {
	Name string         `json:"name"`
	Args []ArgSpec      `json:"args"`
	Type graphQLTypeRef `json:"type"`
}

// Introspect calls the GraphQL endpoint with the introspection query, then builds one tool per root query/mutation.
func Introspect(ctx context.Context, endpoint string, opts Options) ([]toolsy.Tool, error) {
	tools, _, err := IntrospectWithCleanup(ctx, endpoint, opts)
	return tools, err
}

// IntrospectWithCleanup returns tools sharing one owned safe pool and its idle closer.
// Stop new calls before disposal; active calls remain unaffected.
func IntrospectWithCleanup(ctx context.Context, endpoint string, opts Options) ([]toolsy.Tool, func(), error) {
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
	data, err := postIntrospection(ctx, endpoint, opts)
	if err != nil {
		return nil, nil, err
	}
	ir, err := parseIntroResponse(data)
	if err != nil {
		return nil, nil, err
	}
	schema := ir.Data.Schema
	allowedOps := opts.Operations
	if len(allowedOps) == 0 {
		allowedOps = []string{operationQuery, "mutation"}
	}
	allowedSet := make(map[string]bool)
	for _, o := range allowedOps {
		o = strings.ToLower(o)
		if o != "query" && o != "mutation" {
			return nil, nil, fmt.Errorf("graphql: unsupported operation %s", o)
		}
		allowedSet[o] = true
	}
	typeMap, err := buildTypeMap(schema.Types)
	if err != nil {
		return nil, nil, err
	}
	var tools []toolsy.Tool
	usedNames := make(map[string]bool)
	tools, err = appendToolsForOperationKind(
		tools,
		operationQuery,
		"GraphQL query: ",
		schema.QueryType,
		allowedSet[operationQuery],
		typeMap,
		endpoint,
		opts,
		usedNames,
	)
	if err != nil {
		return nil, nil, err
	}
	tools, err = appendToolsForOperationKind(
		tools,
		"mutation",
		"GraphQL mutation: ",
		schema.MutationType,
		allowedSet["mutation"],
		typeMap,
		endpoint,
		opts,
		usedNames,
	)
	if err != nil {
		return nil, nil, err
	}
	success = true
	return tools, client.CloseIdleConnections, nil
}

func postIntrospection(ctx context.Context, endpoint string, opts Options) ([]byte, error) {
	client, owned, clientErr := opts.httpClient()
	if clientErr != nil {
		return nil, clientErr
	}
	if owned {
		defer client.CloseIdleConnections()
	}
	body := map[string]string{operationQuery: introspectionQuery}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("graphql: marshal intro body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("graphql: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.IntrospectionAuthHeader != "" {
		req.Header.Set("Authorization", opts.IntrospectionAuthHeader)
	}
	// #nosec G704 -- endpoint from caller config, not user input
	resp, err := client.Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if err != nil {
		return nil, fmt.Errorf("graphql: introspect: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("graphql: introspection HTTP %d", resp.StatusCode)
	}
	maxBytes := opts.maxResponseBytes()
	data, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, maxBytes)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if toolsy.IsContextInterrupt(err) {
			return nil, err
		}
		if textprocessor.IsReadLimitExceeded(err) {
			return nil, fmt.Errorf("graphql: introspection exceeds %d byte limit: %w", maxBytes, err)
		}
		return nil, fmt.Errorf("graphql: read: %w", err)
	}
	return data, nil
}

func parseIntroResponse(data []byte) (*introResponse, error) {
	if _, err := jsonschemax.Decode(data); err != nil {
		return nil, fmt.Errorf("graphql: invalid introspection JSON: %w", err)
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
		return nil, errors.New("graphql: introspection: no data in response")
	}
	return &ir, nil
}

func buildTypeMap(types []introSchemaType) (map[string]introSchemaType, error) {
	if len(types) > maxContractNodes {
		return nil, errors.New("graphql: type count limit exceeded")
	}
	nodes := 0
	typeMap := make(map[string]introSchemaType)
	for _, t := range types {
		nodes += 1 + len(t.Fields) + len(t.InputFields) + len(t.EnumValues)
		seen := map[string]bool{}
		for _, field := range t.Fields {
			nodes += len(field.Args)
			if seen[field.Name] {
				return nil, errors.New("graphql: duplicate field")
			}
			seen[field.Name] = true
		}
		if nodes > maxContractNodes {
			return nil, errors.New("graphql: schema size limit exceeded")
		}
		if !graphqlName.MatchString(t.Name) {
			return nil, errors.New("graphql: invalid type name")
		}
		if _, exists := typeMap[t.Name]; exists {
			return nil, errors.New("graphql: duplicate type name")
		}
		typeMap[t.Name] = t
	}
	return typeMap, nil
}

func appendToolsForOperationKind(
	tools []toolsy.Tool,
	kind string,
	descPrefix string,
	root *introTypeName,
	allowed bool,
	typeMap map[string]introSchemaType,
	endpoint string,
	opts Options,
	usedNames map[string]bool,
) ([]toolsy.Tool, error) {
	if !allowed || root == nil {
		return tools, nil
	}
	rootType, ok := typeMap[root.Name]
	if !ok {
		return nil, fmt.Errorf("graphql: missing root type %s", root.Name)
	}
	if rootType.Kind != "OBJECT" {
		return nil, errors.New("graphql: root must be object")
	}
	fields := append([]introField(nil), rootType.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, f := range fields {
		if !graphqlName.MatchString(f.Name) {
			return nil, errors.New("graphql: invalid root field name")
		}
		name := toolName(kind+"_"+f.Name, usedNames)
		schemaBytes, err := argsToJSONSchema(f.Args, typeMap)
		if err != nil {
			return nil, fmt.Errorf("graphql: schema %s: %w", f.Name, err)
		}
		selection, output, selectionErr := buildOutputContract(&f.Type, opts.Selections[kind+"."+f.Name], typeMap)
		if selectionErr != nil {
			return nil, fmt.Errorf("graphql: output %s.%s: %w", kind, f.Name, selectionErr)
		}
		queryText, queryErr := buildStaticQuery(kind, f.Name, f.Args, selection)
		if queryErr != nil {
			return nil, queryErr
		}
		endpointCopy := endpoint
		optsCopy := opts
		tool, err := toolsy.NewProxyTool(
			name,
			descPrefix+f.Name,
			schemaBytes,
			func(ctx context.Context, run *toolsy.RunEnv, argsJSON []byte, yield func(toolsy.Chunk) error) error {
				return executeGraphQL(ctx, run, name, endpointCopy, queryText, f.Name, argsJSON, &optsCopy, yield)
			},
			toolsy.WithOutputSchema(output),
		)
		if err != nil {
			return nil, fmt.Errorf("graphql: tool %s: %w", name, err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func executeGraphQL(
	ctx context.Context,
	run *toolsy.RunEnv,
	toolName string,
	endpoint, queryText, rootField string,
	argsJSON []byte,
	opts *Options,
	yield func(toolsy.Chunk) error,
) error {
	var variables map[string]json.RawMessage
	if len(argsJSON) > 0 {
		if err := json.Unmarshal(argsJSON, &variables); err != nil {
			return fmt.Errorf("graphql: variables: %w", err)
		}
	}
	if variables == nil {
		variables = make(map[string]json.RawMessage)
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
	if run.Credentials != nil {
		authHeader, authErr := run.Credentials.GetAuth(ctx, toolName)
		if authErr != nil {
			return fmt.Errorf("graphql: credentials for %s: %w", toolName, authErr)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}
	// #nosec G704 -- endpoint from caller config, not user input
	client, owned, clientErr := opts.httpClient()
	if clientErr != nil {
		return clientErr
	}
	if owned {
		defer client.CloseIdleConnections()
	}
	resp, err := client.Do(req) //nolint:bodyclose // closed via httptool.CloseResponseBody
	if err != nil {
		return fmt.Errorf("graphql: do: %w", err)
	}
	defer httptool.CloseResponseBody(ctx, resp.Body)
	if !httptool.IsSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("graphql: response status %d", resp.StatusCode)
	}
	data, err := textprocessor.ReadLimitedBytes(ctx, resp.Body, opts.maxResponseBytes())
	if err != nil {
		return fmt.Errorf("graphql: read: %w", err)
	}
	if _, decodeErr := jsonschemax.Decode(data); decodeErr != nil {
		return fmt.Errorf("graphql: invalid response JSON: %w", decodeErr)
	}
	var response struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []introError               `json:"errors"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("graphql: response: %w", err)
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("graphql: execution errors: %s", response.Errors[0].Message)
	}
	value, ok := response.Data[rootField]
	if !ok {
		return errors.New("graphql: missing root result")
	}
	return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: value, MimeType: toolsy.MimeTypeJSON})
}
