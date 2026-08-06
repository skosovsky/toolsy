//nolint:exhaustruct // Wire DTO and state constructors intentionally spell only negotiated/non-zero fields.
package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"math"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/jsonschemax"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	progressChunkBufferSize   = 8
	cancelNotifyTimeout       = 5 * time.Second
	cancelDeliveryWaitTimeout = time.Second
	clientEventBufferSize     = 64
	maxCancellationRunes      = 256
	structuredContentSubject  = "tool structuredContent"
	fileScheme                = "file"
)

type ClientOption func(*ClientOptions)

type ClientOptions struct {
	Roots      []Root
	RootPaths  []string
	Logger     *slog.Logger
	ClientInfo Implementation
	Pagination PaginationLimits
}

func WithClientRoots(paths []string) ClientOption {
	return func(options *ClientOptions) { options.RootPaths = append([]string(nil), paths...) }
}

func WithRoots(roots []Root) ClientOption {
	snapshot := cloneRoots(roots)
	return func(options *ClientOptions) { options.Roots = cloneRoots(snapshot) }
}

func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(options *ClientOptions) { options.Logger = logger }
}

func WithClientInfo(info Implementation) ClientOption {
	return func(options *ClientOptions) { options.ClientInfo = info }
}

func WithPaginationLimits(limits PaginationLimits) ClientOption {
	return func(options *ClientOptions) { options.Pagination = limits.normalized() }
}

type InvalidationKind string

const (
	InvalidationTools     InvalidationKind = capabilityToolsField
	InvalidationResources InvalidationKind = capabilityResourcesField
	InvalidationResource  InvalidationKind = "resource"
	InvalidationPrompts   InvalidationKind = capabilityPromptsField
)

type Invalidation struct {
	Kind       InvalidationKind
	URI        string
	Generation uint64
}

type progressState struct {
	mu      sync.Mutex
	last    float64
	started bool
	ch      chan<- toolsy.Chunk
	done    <-chan struct{}
}

type Client struct {
	transport Transport
	opts      ClientOptions
	logger    *slog.Logger

	mu          sync.RWMutex
	initialized bool
	server      InitializeResult
	roots       []Root
	subscribed  map[string]struct{}

	progressCounter    atomic.Uint64
	progressCallbacks  sync.Map
	invalidationCount  atomic.Uint64
	toolGeneration     atomic.Uint64
	resourceGeneration atomic.Uint64
	promptGeneration   atomic.Uint64
	invalidations      chan Invalidation
	logMessages        chan LogMessageParams
	closeOnce          sync.Once
	closeErr           error
}

func Connect(ctx context.Context, transport Transport, opts ...ClientOption) (*Client, error) {
	options := ClientOptions{
		ClientInfo: Implementation{Name: "toolsy-mcp-client", Version: "0.6.0"},
	}
	for _, opt := range opts {
		opt(&options)
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if err := validateImplementation(options.ClientInfo); err != nil {
		return nil, &InvalidPayloadError{Subject: "clientInfo", Err: err}
	}
	roots, err := normalizeRoots(options.RootPaths, options.Roots)
	if err != nil {
		return nil, err
	}
	client := &Client{
		transport:     transport,
		opts:          options,
		logger:        options.Logger,
		roots:         roots,
		subscribed:    make(map[string]struct{}),
		invalidations: make(chan Invalidation, clientEventBufferSize),
		logMessages:   make(chan LogMessageParams, clientEventBufferSize),
	}
	if err := transport.Start(ctx); err != nil {
		return nil, err
	}
	client.registerHandlers()
	if err := client.initialize(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}
	return client, nil
}

func normalizeRoots(paths []string, roots []Root) ([]Root, error) {
	result := cloneRoots(roots)
	for _, rootPath := range paths {
		root, err := rootFromPath(rootPath)
		if err != nil {
			return nil, err
		}
		result = append(result, root)
	}
	seen := make(map[string]struct{}, len(result))
	for index := range result {
		canonical, err := canonicalRootURI(result[index].URI)
		if err != nil {
			return nil, err
		}
		result[index].URI = canonical
		if _, exists := seen[result[index].URI]; exists {
			return nil, fmt.Errorf("mcp: duplicate root URI %q", result[index].URI)
		}
		seen[result[index].URI] = struct{}{}
	}
	return result, nil
}

func cloneRoots(roots []Root) []Root {
	if roots == nil {
		return nil
	}
	result := make([]Root, len(roots))
	for index, root := range roots {
		result[index] = root
		result[index].Meta = cloneMeta(root.Meta)
	}
	return result
}

func cloneMeta(meta Meta) Meta {
	if meta == nil {
		return nil
	}
	result := make(Meta, len(meta))
	for name, raw := range meta {
		result[name] = bytes.Clone(raw)
	}
	return result
}

func rootFromPath(rootPath string) (Root, error) {
	if strings.HasPrefix(rootPath, "file://") {
		return Root{URI: rootPath}, nil
	}
	if strings.HasPrefix(rootPath, `\\`) || strings.HasPrefix(rootPath, "//") {
		return Root{}, fmt.Errorf("mcp: UNC root %q is not supported", rootPath)
	}
	if windowsDriveRootPattern.MatchString(rootPath) {
		slashed := strings.ReplaceAll(rootPath, `\`, "/")
		uriPath := cleanWindowsDriveURIPath("/" + slashed)
		return Root{
			URI:  (&url.URL{Scheme: fileScheme, Path: uriPath}).String(),
			Name: windowsRootName(uriPath),
		}, nil
	}
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return Root{}, fmt.Errorf("mcp: normalize root %q: %w", rootPath, err)
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return Root{
		URI:  (&url.URL{Scheme: fileScheme, Path: uriPath}).String(),
		Name: filepath.Base(absolute),
	}, nil
}

func canonicalRootURI(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != fileScheme || parsed.Host != "" || parsed.Path == "" ||
		!strings.HasPrefix(parsed.Path, "/") || parsed.OmitHost || parsed.Opaque != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		isUNCFileURIPath(parsed.Path) {
		return "", fmt.Errorf("mcp: root URI %q must be an absolute file:// URI", rawURI)
	}
	cleanedPath := pathpkg.Clean(parsed.Path)
	if windowsDriveURIPathPrefixPattern.MatchString(parsed.Path) {
		if !windowsDriveURIPathPattern.MatchString(parsed.Path) {
			return "", fmt.Errorf("mcp: root URI %q must use an absolute Windows drive path", rawURI)
		}
		cleanedPath = cleanWindowsDriveURIPath(parsed.Path)
	}
	return (&url.URL{Scheme: fileScheme, Path: cleanedPath}).String(), nil
}

func cleanWindowsDriveURIPath(uriPath string) string {
	normalized := strings.ReplaceAll(uriPath, `\`, "/")
	drive := strings.ToUpper(normalized[1:2]) + ":"
	tail := strings.TrimLeft(normalized[3:], "/")
	cleanedTail := pathpkg.Clean("/" + tail)
	if cleanedTail == "/" {
		return "/" + drive + "/"
	}
	return "/" + drive + cleanedTail
}

func isUNCFileURIPath(uriPath string) bool {
	normalized := strings.ReplaceAll(uriPath, `\`, "/")
	return strings.HasPrefix(normalized, "//")
}

func windowsRootName(uriPath string) string {
	if windowsDriveRootURIPathPattern.MatchString(strings.TrimSuffix(uriPath, "/")) {
		return uriPath[1:3]
	}
	return pathpkg.Base(uriPath)
}

var (
	windowsDriveRootPattern          = regexp.MustCompile(`^[A-Za-z]:[\\/].*`)
	windowsDriveURIPathPrefixPattern = regexp.MustCompile(`^/[A-Za-z]:`)
	windowsDriveURIPathPattern       = regexp.MustCompile(`^/[A-Za-z]:[\\/]`)
	windowsDriveRootURIPathPattern   = regexp.MustCompile(`^/[A-Za-z]:$`)
)

func (c *Client) registerHandlers() {
	c.transport.OnRequest(c.handleRequest)
	c.transport.OnNotification(MethodProgress, c.handleProgress)
	c.transport.OnNotification(
		MethodToolsListChanged,
		func(params json.RawMessage) { c.handleListChanged(InvalidationTools, params) },
	)
	c.transport.OnNotification(
		MethodResourcesListChanged,
		func(params json.RawMessage) { c.handleListChanged(InvalidationResources, params) },
	)
	c.transport.OnNotification(
		MethodPromptsListChanged,
		func(params json.RawMessage) { c.handleListChanged(InvalidationPrompts, params) },
	)
	c.transport.OnNotification(MethodResourceUpdated, c.handleResourceUpdated)
	c.transport.OnNotification(MethodLogMessage, c.handleLogMessage)
}

func (c *Client) initialize(ctx context.Context) error {
	capabilities := ClientCapabilities{}
	if len(c.roots) > 0 {
		capabilities.Roots = &RootsCapability{ListChanged: false}
	}
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    capabilities,
		ClientInfo:      c.opts.ClientInfo,
	}
	resultRaw, err := c.requestAndAwait(ctx, MethodInitialize, params)
	if err != nil {
		return c.mapCallReadLimitFor(ctx, err, "MCP initialize response")
	}
	var result InitializeResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return &InvalidPayloadError{Subject: "initialize result", Err: err}
	}
	if result.ProtocolVersion != ProtocolVersion {
		return &ProtocolVersionError{Requested: ProtocolVersion, Selected: result.ProtocolVersion}
	}
	if err := validateImplementation(result.ServerInfo); err != nil {
		return &InvalidPayloadError{Subject: serverInfoField, Err: err}
	}
	if versioned, ok := c.transport.(ProtocolVersionTransport); ok {
		versioned.SetProtocolVersion(result.ProtocolVersion)
	}
	if err := c.transport.Notify(ctx, MethodInitialized, struct{}{}); err != nil {
		return fmt.Errorf("mcp: send initialized notification: %w", err)
	}
	c.mu.Lock()
	c.server = result
	c.initialized = true
	c.mu.Unlock()
	if transport, ok := c.transport.(OperationPhaseTransport); ok {
		transport.Activate()
	}
	return nil
}

func validateImplementation(info Implementation) error {
	return validateIcons(info.Icons)
}

func validateIcons(icons []Icon) error {
	for _, icon := range icons {
		if icon.Src == "" {
			return errors.New("icon src is required")
		}
		if icon.Theme != "" && icon.Theme != "light" && icon.Theme != "dark" {
			return fmt.Errorf("invalid icon theme %q", icon.Theme)
		}
	}
	return nil
}

func (c *Client) handleRequest(
	_ context.Context,
	request Request,
) (json.RawMessage, *JSONRPCError) {
	if request.Method == MethodPing || request.Method == MethodRootsList {
		if err := validateInboundRequestParams(request.Params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: "Invalid params"}
		}
	}
	if request.Method == MethodPing {
		return json.RawMessage(`{}`), nil
	}
	c.mu.RLock()
	initialized := c.initialized
	c.mu.RUnlock()
	if !initialized {
		return nil, &JSONRPCError{Code: JSONRPCMethodNotFound, Message: rpcMethodNotFoundMessage}
	}
	switch request.Method {
	case MethodRootsList:
		if len(c.roots) == 0 {
			return nil, &JSONRPCError{
				Code:    JSONRPCMethodNotFound,
				Message: rpcMethodNotFoundMessage,
			}
		}
		result, err := json.Marshal(RootsListResult{Roots: append([]Root(nil), c.roots...)})
		if err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInternalError, Message: "Internal error"}
		}
		return result, nil
	default:
		return nil, &JSONRPCError{Code: JSONRPCMethodNotFound, Message: rpcMethodNotFoundMessage}
	}
}

func validateInboundRequestParams(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var params RequestParams
	return json.Unmarshal(raw, &params)
}

func (c *Client) requireCapability(name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.initialized {
		return &CapabilityError{Capability: "initialized"}
	}
	switch name {
	case "tools":
		if c.server.Capabilities.Tools == nil {
			return &CapabilityError{Capability: name}
		}
	case "resources":
		if c.server.Capabilities.Resources == nil {
			return &CapabilityError{Capability: name}
		}
	case "prompts":
		if c.server.Capabilities.Prompts == nil {
			return &CapabilityError{Capability: name}
		}
	}
	return nil
}

func (c *Client) requestAndAwait(
	ctx context.Context,
	method string,
	params any,
) (json.RawMessage, error) {
	result, _, err := c.requestAndAwaitWithID(ctx, method, params)
	return result, err
}

func (c *Client) requestAndAwaitWithID(
	ctx context.Context,
	method string,
	params any,
) (json.RawMessage, json.RawMessage, error) {
	pending, err := c.transport.Request(ctx, method, params)
	if err != nil {
		return nil, nil, err
	}
	requestID := pending.ID()
	result, err := pending.Await(ctx)
	if err != nil && ctx.Err() != nil && method != MethodInitialize {
		c.notifyCancelledPending(ctx, pending, ctx.Err().Error())
	}
	return result, requestID, err
}

func (c *Client) maxStreamBytes() int {
	if transport, ok := c.transport.(StreamByteCapTransport); ok && transport.MaxStreamBytes() > 0 {
		return transport.MaxStreamBytes()
	}
	return httptool.DefaultMaxSSEStreamBytes
}

func (c *Client) mapCallReadLimitFor(ctx context.Context, err error, subject string) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if toolsy.IsContextInterrupt(err) {
		return err
	}
	if textprocessor.IsReadLimitExceeded(err) {
		return toolsy.MapReadLimitErrorFor(err, c.maxStreamBytes(), subject, "")
	}
	return err
}

func (c *Client) GetTools(ctx context.Context) iter.Seq2[toolsy.Tool, error] {
	if err := c.requireCapability("tools"); err != nil {
		return errorSequence[toolsy.Tool](err)
	}
	discoveredGeneration := c.toolGeneration.Load()
	fetch := func(ctx context.Context, cursor string) ([]MCPTool, string, error) {
		if current := c.toolGeneration.Load(); current != discoveredGeneration {
			return nil, "", &StaleDiscoveryError{
				Kind:       InvalidationTools,
				Discovered: discoveredGeneration,
				Current:    current,
			}
		}
		resultRaw, err := c.requestAndAwait(ctx, MethodToolsList, ToolsListParams{Cursor: cursor})
		if err != nil {
			return nil, "", c.mapCallReadLimitFor(ctx, err, "MCP tools list response")
		}
		var result ToolsListResult
		if err := json.Unmarshal(resultRaw, &result); err != nil {
			return nil, "", &InvalidPayloadError{Subject: "tools/list result", Err: err}
		}
		if current := c.toolGeneration.Load(); current != discoveredGeneration {
			return nil, "", &StaleDiscoveryError{
				Kind:       InvalidationTools,
				Discovered: discoveredGeneration,
				Current:    current,
			}
		}
		return result.Tools, result.NextCursor, nil
	}
	return func(yield func(toolsy.Tool, error) bool) {
		for descriptor, err := range IterateCursorWithLimits(ctx, c.opts.Pagination, fetch) {
			if err != nil {
				yield(nil, err)
				return
			}
			proxy, err := c.toolToProxyAtGeneration(descriptor, discoveredGeneration)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(proxy, nil) {
				return
			}
		}
	}
}

func errorSequence[T any](err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

var mcpToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func (c *Client) toolToProxy(descriptor MCPTool) (toolsy.Tool, error) {
	return c.toolToProxyAtGeneration(descriptor, c.toolGeneration.Load())
}

func (c *Client) toolToProxyAtGeneration(
	descriptor MCPTool,
	discoveredGeneration uint64,
) (toolsy.Tool, error) {
	if err := validateMCPTool(descriptor); err != nil {
		return nil, &InvalidPayloadError{Subject: "tool descriptor", Err: err}
	}
	if !mcpToolNamePattern.MatchString(descriptor.Name) {
		return nil, &InvalidPayloadError{
			Subject: "tool name",
			Err:     fmt.Errorf("invalid name %q", descriptor.Name),
		}
	}
	if descriptor.Execution != nil {
		switch descriptor.Execution.TaskSupport {
		case "", taskSupportOptional, taskSupportForbidden:
		case taskSupportRequired:
			return nil, &UnsupportedFeatureError{
				Feature: "tasks required by tool " + descriptor.Name,
			}
		default:
			return nil, &InvalidPayloadError{
				Subject: "tool execution",
				Err:     fmt.Errorf("unknown taskSupport %q", descriptor.Execution.TaskSupport),
			}
		}
	}
	inputSchema, err := decodeSchemaObject(descriptor.InputSchema, true)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: "tool inputSchema", Err: err}
	}
	outputSchema, err := decodeSchemaObject(descriptor.OutputSchema, false)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: "tool outputSchema", Err: err}
	}
	validator, err := compileOutputSchema(descriptor.OutputSchema)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: "tool outputSchema", Err: err}
	}
	description := descriptor.Description
	if description == "" {
		description = descriptor.Title
	}
	if description == "" && descriptor.Annotations != nil {
		description = descriptor.Annotations.Title
	}
	if description == "" {
		description = descriptor.Name
	}
	schemaJSON, _ := json.Marshal(inputSchema)
	handler := func(ctx context.Context, _ *toolsy.RunEnv, rawArgs []byte, yield func(toolsy.Chunk) error) error {
		if current := c.toolGeneration.Load(); current != discoveredGeneration {
			return &StaleDiscoveryError{
				Kind:       InvalidationTools,
				Discovered: discoveredGeneration,
				Current:    current,
			}
		}
		return c.runMCPToolCall(ctx, descriptor.Name, rawArgs, validator, yield)
	}
	options := mcpToolPolicyOptions(descriptor.Annotations)
	if outputSchema != nil {
		options = append(options, toolsy.WithOutputSchema(outputSchema))
	}
	return toolsy.NewProxyTool(descriptor.Name, description, schemaJSON, handler, options...)
}

func validateMCPTool(descriptor MCPTool) error {
	return validateIcons(descriptor.Icons)
}

func validatePrompt(prompt Prompt) error {
	return validateIcons(prompt.Icons)
}

func decodeSchemaObject(raw json.RawMessage, required bool) (map[string]any, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		if required {
			return nil, errors.New("schema is required and must not be null")
		}
		return nil, nil //nolint:nilnil // An absent optional output schema is represented by nil, nil.
	}
	decoded, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	schema, ok := decoded.(map[string]any)
	if !ok || schema == nil {
		return nil, errors.New("schema must be a JSON object")
	}
	if schemaType, ok := schema["type"]; !ok || schemaType != "object" {
		return nil, errors.New("schema root type must be object")
	}
	if _, err := jsonschemax.Compile(schema); err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

func compileOutputSchema(raw json.RawMessage) (schemaValidator, error) {
	if len(raw) == 0 || string(raw) == jsonNull {
		return nil, nil //nolint:nilnil // No validator is the valid representation of an absent schema.
	}
	schema, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	return jsonschemax.Compile(schema)
}

type schemaValidator interface {
	Validate(any) error
}

//nolint:gocognit // Select loop keeps response/progress/cancellation ordering explicit.
func (c *Client) runMCPToolCall(
	ctx context.Context,
	name string,
	rawArgs []byte,
	outputSchema schemaValidator,
	yield func(toolsy.Chunk) error,
) error {
	token := NewStringProgressToken(c.nextProgressToken())
	progressCh := make(chan toolsy.Chunk, progressChunkBufferSize)
	done := make(chan struct{})
	state := &progressState{ch: progressCh, done: done}
	c.progressCallbacks.Store(token.String(), state)
	defer func() {
		c.progressCallbacks.Delete(token.String())
		close(done)
	}()

	pending, err := c.transport.Request(ctx, MethodToolsCall, ToolsCallParams{
		Name:      name,
		Arguments: json.RawMessage(rawArgs),
		Meta:      &RequestMeta{ProgressToken: token},
	})
	if err != nil {
		return err
	}
	if completable, ok := pending.(CompletionPendingRequest); ok {
		completable.OnComplete(func() { c.progressCallbacks.Delete(token.String()) })
	}
	type finalResult struct {
		result json.RawMessage
		err    error
	}
	finalCh := make(chan finalResult, 1)
	go func() {
		result, awaitErr := pending.Await(ctx)
		finalCh <- finalResult{result: result, err: awaitErr}
	}()

	for {
		select {
		case <-ctx.Done():
			c.notifyCancelledPending(ctx, pending, ctx.Err().Error())
			return ctx.Err()
		case progress := <-progressCh:
			if err := yield(progress); err != nil {
				c.notifyCancelledPending(ctx, pending, "result consumer aborted")
				return toolsy.ErrStreamAborted
			}
		case final := <-finalCh:
			for {
				select {
				case progress := <-progressCh:
					if err := yield(progress); err != nil {
						c.notifyCancelledPending(ctx, pending, "result consumer aborted")
						return toolsy.ErrStreamAborted
					}
				default:
					goto progressDrained
				}
			}
		progressDrained:
			if final.err != nil {
				if ctx.Err() != nil {
					c.notifyCancelledPending(ctx, pending, ctx.Err().Error())
					return ctx.Err()
				}
				return c.mapCallReadLimitFor(ctx, final.err, "MCP tool response")
			}
			chunk, err := buildToolResultChunk(name, final.result, outputSchema)
			if err != nil {
				return err
			}
			if err := yield(chunk); err != nil {
				c.notifyCancelledPending(ctx, pending, "result consumer aborted")
				return toolsy.ErrStreamAborted
			}
			return nil
		}
	}
}

func buildToolResultChunk(
	name string,
	raw json.RawMessage,
	outputSchema schemaValidator,
) (toolsy.Chunk, error) {
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return toolsy.Chunk{}, &InvalidPayloadError{Subject: "tools/call result", Err: err}
	}
	projection, err := formatContentBlocks(result.Content)
	if err != nil {
		return toolsy.Chunk{}, &InvalidPayloadError{Subject: "tool content", Err: err}
	}
	if result.IsError {
		remoteErr := &RemoteToolError{ToolName: name, Result: result}
		toolErr := &toolsy.ToolError{
			Code:   toolsy.CodeRemoteExecution,
			Reason: "remote tool execution failed",
			Err:    remoteErr,
		}
		return toolsy.Chunk{
			Event:       toolsy.EventResult,
			Data:        projection,
			MimeType:    toolsy.MimeTypeText,
			IsError:     true,
			TypedResult: result,
			Envelope: toolsy.NewErrorEnvelope(
				toolErr,
				projection,
				toolsy.MimeTypeText,
				toolsy.DeliveryClassStructured,
				toolsy.AudienceModel,
				nil,
			),
		}, nil
	}
	if outputSchema != nil && len(result.StructuredContent) == 0 {
		return toolsy.Chunk{}, &InvalidPayloadError{
			Subject: structuredContentSubject,
			Err:     errors.New("outputSchema requires structuredContent"),
		}
	}
	//nolint:nestif // Structured validation is intentionally fail-closed in one branch.
	if len(
		result.StructuredContent,
	) > 0 {
		canonical, value, err := canonicalJSON(result.StructuredContent)
		if err != nil {
			return toolsy.Chunk{}, &InvalidPayloadError{Subject: structuredContentSubject, Err: err}
		}
		if _, ok := value.(map[string]any); !ok {
			return toolsy.Chunk{}, &InvalidPayloadError{
				Subject: structuredContentSubject,
				Err:     errors.New("must be a JSON object"),
			}
		}
		if outputSchema != nil {
			if err := outputSchema.Validate(value); err != nil {
				return toolsy.Chunk{}, &InvalidPayloadError{
					Subject: structuredContentSubject,
					Err:     err,
				}
			}
		}
		return toolsy.Chunk{
			Event:       toolsy.EventResult,
			Data:        canonical,
			MimeType:    toolsy.MimeTypeJSON,
			TypedResult: result,
			Envelope: toolsy.NewResultEnvelope(
				result,
				canonical,
				toolsy.MimeTypeJSON,
				toolsy.DeliveryClassStructured,
				toolsy.AudienceModel,
				nil,
			),
		}, nil
	}
	return toolsy.Chunk{
		Event:       toolsy.EventResult,
		Data:        projection,
		MimeType:    toolsy.MimeTypeText,
		TypedResult: result,
		EmptyResult: len(projection) == 0,
		Envelope: toolsy.NewResultEnvelope(
			result,
			projection,
			toolsy.MimeTypeText,
			toolsy.DeliveryClassText,
			toolsy.AudienceModel,
			nil,
		),
	}, nil
}

func (c *Client) nextProgressToken() string {
	return fmt.Sprintf("progress-%d-%d", time.Now().UnixNano(), c.progressCounter.Add(1))
}

func (c *Client) handleProgress(params json.RawMessage) {
	fields, fieldsErr := decodeObjectFields(params)
	if fieldsErr != nil {
		c.logger.Warn("mcp: invalid progress notification", "err", fieldsErr)
		return
	}
	if err := requireNumberField(fields, "progress"); err != nil {
		c.logger.Warn("mcp: invalid progress notification", "err", err)
		return
	}
	if _, hasTotal := fields["total"]; hasTotal {
		if err := requireNumberField(fields, "total"); err != nil {
			c.logger.Warn("mcp: invalid progress notification", "err", err)
			return
		}
	}
	if _, hasMessage := fields["message"]; hasMessage {
		if err := requireStringField(fields, "message", true); err != nil {
			c.logger.Warn("mcp: invalid progress notification", "err", err)
			return
		}
	}
	var progress ProgressParams
	if err := json.Unmarshal(params, &progress); err != nil {
		c.logger.Warn("mcp: invalid progress notification", "err", err)
		return
	}
	if progress.ProgressToken.IsZero() || math.IsNaN(progress.Progress) ||
		math.IsInf(progress.Progress, 0) ||
		(progress.Total != nil && (math.IsNaN(*progress.Total) || math.IsInf(*progress.Total, 0))) {
		c.logger.Warn("mcp: invalid progress notification values")
		return
	}
	value, ok := c.progressCallbacks.Load(progress.ProgressToken.String())
	if !ok {
		return
	}
	state, ok := value.(*progressState)
	if !ok {
		return
	}
	state.mu.Lock()
	if state.started && progress.Progress <= state.last {
		state.mu.Unlock()
		c.logger.Warn(
			"mcp: non-monotonic progress ignored",
			"token",
			boundedDiagnostic(progress.ProgressToken.String()),
		)
		return
	}
	state.started = true
	state.last = progress.Progress
	state.mu.Unlock()
	info := &toolsy.ProgressInfo{
		Current: &progress.Progress,
		Total:   progress.Total,
		Message: progress.Message,
		Token:   progress.ProgressToken.String(),
	}
	select {
	case state.ch <- toolsy.Chunk{Event: toolsy.EventProgress, Progress: info}:
	case <-state.done:
	default:
		c.logger.Warn(
			"mcp: progress notification dropped because consumer is slow",
			"token",
			boundedDiagnostic(progress.ProgressToken.String()),
		)
	}
}

func (c *Client) notifyCancelledRequest(
	parent context.Context,
	requestID json.RawMessage,
	reason string,
) {
	if len(requestID) == 0 {
		return
	}
	reason = strings.Map(func(char rune) rune {
		if char == '\n' || char == '\r' || char == '\t' {
			return ' '
		}
		return char
	}, reason)
	reasonRunes := []rune(reason)
	if len(reasonRunes) > maxCancellationRunes {
		reason = string(reasonRunes[:maxCancellationRunes])
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), cancelNotifyTimeout)
	defer cancel()
	if err := c.transport.Notify(
		ctx,
		MethodCancelled,
		CancelledParams{RequestID: requestID, Reason: reason},
	); err != nil {
		c.logger.WarnContext(ctx, "mcp: cancellation notification failed", "err", err)
	}
}

func (c *Client) notifyCancelledPending(
	parent context.Context,
	pending PendingRequest,
	reason string,
) {
	if cancellable, ok := pending.(CancellablePendingRequest); ok && !cancellable.CancelPending() {
		return
	}
	if delivery, ok := pending.(DeliveryPendingRequest); ok {
		timer := time.NewTimer(cancelDeliveryWaitTimeout)
		select {
		case <-delivery.DeliveryDone():
			timer.Stop()
		case <-timer.C:
			return
		}
		if !delivery.WasSent() {
			return
		}
	}
	c.notifyCancelledRequest(parent, pending.ID(), reason)
}

func (c *Client) GetResourceTool() (toolsy.Tool, error) {
	if err := c.requireCapability("resources"); err != nil {
		return nil, err
	}
	schema := []byte(
		`{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"],"additionalProperties":false}`,
	)
	handler := func(ctx context.Context, _ *toolsy.RunEnv, argsJSON []byte, yield func(toolsy.Chunk) error) error {
		var args struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return err
		}
		resultRaw, err := c.requestAndAwait(
			ctx,
			MethodResourcesRead,
			ResourcesReadParams{URI: args.URI},
		)
		if err != nil {
			return c.mapCallReadLimitFor(ctx, err, "MCP resource read response")
		}
		var result ResourcesReadResult
		if decodeErr := json.Unmarshal(resultRaw, &result); decodeErr != nil {
			return &InvalidPayloadError{Subject: "resources/read result", Err: decodeErr}
		}
		projection, err := formatResourceContents(result.Contents)
		if err != nil {
			return &InvalidPayloadError{Subject: "resource contents", Err: err}
		}
		chunk, err := buildResourceResultChunk(result, projection)
		if err != nil {
			return err
		}
		if err := yield(chunk); err != nil {
			return toolsy.ErrStreamAborted
		}
		return nil
	}
	return toolsy.NewProxyTool(
		"read_mcp_resource",
		"Reads a resource by URI from the MCP server",
		schema,
		handler,
		toolsy.WithReadOnly(),
	)
}

func buildResourceResultChunk(result ResourcesReadResult, projection []byte) (toolsy.Chunk, error) {
	data := projection
	mimeType := toolsy.MimeTypeText
	delivery := toolsy.DeliveryClassText
	//nolint:nestif // Single-resource MIME projection has explicit text/blob branches.
	if len(
		result.Contents,
	) == 1 {
		content := result.Contents[0]
		mimeType = content.MIMEType
		if content.Text != nil {
			data = []byte(*content.Text)
			if mimeType == "" {
				mimeType = toolsy.MimeTypeText
			}
			if mimeType == toolsy.MimeTypeJSON {
				delivery = toolsy.DeliveryClassStructured
			}
		} else if content.Blob != nil {
			decoded, err := base64.StdEncoding.DecodeString(*content.Blob)
			if err != nil {
				return toolsy.Chunk{}, &InvalidPayloadError{Subject: "resource blob", Err: err}
			}
			data = decoded
			if mimeType == "" {
				mimeType = applicationOctetStream
			}
			delivery = toolsy.DeliveryClassBinary
		}
	}
	return toolsy.Chunk{
		Event:       toolsy.EventResult,
		Data:        data,
		MimeType:    mimeType,
		TypedResult: result,
		EmptyResult: len(result.Contents) == 0,
		Envelope: toolsy.NewResultEnvelope(
			result,
			data,
			mimeType,
			delivery,
			toolsy.AudienceModel,
			nil,
		),
	}, nil
}

func (c *Client) SubscribeResource(ctx context.Context, uri string) error {
	if err := c.requireCapability("resources"); err != nil {
		return err
	}
	c.mu.RLock()
	canSubscribe := c.server.Capabilities.Resources.Subscribe
	c.mu.RUnlock()
	if !canSubscribe {
		return &UnsupportedFeatureError{Feature: "resource subscriptions"}
	}
	resultRaw, err := c.requestAndAwait(ctx, MethodResourcesSubscribe, ResourcesReadParams{URI: uri})
	if err != nil {
		return err
	}
	var result EmptyResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return &InvalidPayloadError{Subject: "resources/subscribe result", Err: err}
	}
	c.mu.Lock()
	c.subscribed[uri] = struct{}{}
	c.mu.Unlock()
	return nil
}

func (c *Client) GetPrompts(ctx context.Context) iter.Seq2[Prompt, error] {
	if err := c.requireCapability("prompts"); err != nil {
		return errorSequence[Prompt](err)
	}
	discoveredGeneration := c.promptGeneration.Load()
	fetch := func(ctx context.Context, cursor string) ([]Prompt, string, error) {
		if current := c.promptGeneration.Load(); current != discoveredGeneration {
			return nil, "", &StaleDiscoveryError{
				Kind:       InvalidationPrompts,
				Discovered: discoveredGeneration,
				Current:    current,
			}
		}
		resultRaw, err := c.requestAndAwait(
			ctx,
			MethodPromptsList,
			PromptsListParams{Cursor: cursor},
		)
		if err != nil {
			return nil, "", c.mapCallReadLimitFor(ctx, err, "MCP prompts list response")
		}
		var result PromptsListResult
		if err := json.Unmarshal(resultRaw, &result); err != nil {
			return nil, "", &InvalidPayloadError{Subject: "prompts/list result", Err: err}
		}
		if current := c.promptGeneration.Load(); current != discoveredGeneration {
			return nil, "", &StaleDiscoveryError{
				Kind:       InvalidationPrompts,
				Discovered: discoveredGeneration,
				Current:    current,
			}
		}
		for _, prompt := range result.Prompts {
			if err := validatePrompt(prompt); err != nil {
				return nil, "", &InvalidPayloadError{
					Subject: "prompt descriptor",
					Err:     err,
				}
			}
		}
		return result.Prompts, result.NextCursor, nil
	}
	return IterateCursorWithLimits(ctx, c.opts.Pagination, fetch)
}

func (c *Client) GetPrompt(
	ctx context.Context,
	name string,
	args map[string]string,
) (*PromptsGetResult, error) {
	if err := c.requireCapability("prompts"); err != nil {
		return nil, err
	}
	resultRaw, err := c.requestAndAwait(
		ctx,
		MethodPromptsGet,
		PromptsGetParams{Name: name, Arguments: args},
	)
	if err != nil {
		return nil, c.mapCallReadLimitFor(ctx, err, "MCP prompt response")
	}
	var result PromptsGetResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return nil, &InvalidPayloadError{Subject: "prompts/get result", Err: err}
	}
	for _, message := range result.Messages {
		if _, err := formatContentBlocks([]ContentBlock{message.Content}); err != nil {
			return nil, &InvalidPayloadError{Subject: "prompt content", Err: err}
		}
	}
	return &result, nil
}

func (c *Client) emitInvalidation(kind InvalidationKind, uri string) {
	if !c.invalidationAllowed(kind) {
		c.logger.Warn("mcp: invalidation contradicts negotiated capability", "kind", kind)
		return
	}
	generation := c.invalidationCount.Add(1)
	switch kind {
	case InvalidationTools:
		c.toolGeneration.Add(1)
	case InvalidationResources, InvalidationResource:
		c.resourceGeneration.Add(1)
	case InvalidationPrompts:
		c.promptGeneration.Add(1)
	}
	event := Invalidation{Kind: kind, URI: uri, Generation: generation}
	select {
	case c.invalidations <- event:
	default:
		c.logger.Warn(
			"mcp: invalidation event channel full",
			"kind",
			kind,
			"generation",
			generation,
		)
	}
}

func (c *Client) handleListChanged(kind InvalidationKind, params json.RawMessage) {
	if len(params) > 0 {
		var decoded NotificationParams
		if err := json.Unmarshal(params, &decoded); err != nil {
			c.logger.Warn("mcp: invalid list-changed notification", "kind", kind, "err", err)
			return
		}
	}
	c.emitInvalidation(kind, "")
}

func (c *Client) invalidationAllowed(kind InvalidationKind) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch kind {
	case InvalidationTools:
		return c.server.Capabilities.Tools != nil && c.server.Capabilities.Tools.ListChanged
	case InvalidationResources:
		return c.server.Capabilities.Resources != nil && c.server.Capabilities.Resources.ListChanged
	case InvalidationPrompts:
		return c.server.Capabilities.Prompts != nil && c.server.Capabilities.Prompts.ListChanged
	case InvalidationResource:
		return c.server.Capabilities.Resources != nil && c.server.Capabilities.Resources.Subscribe
	default:
		return false
	}
}

func (c *Client) handleResourceUpdated(params json.RawMessage) {
	var updated ResourceUpdatedParams
	if err := json.Unmarshal(params, &updated); err != nil {
		c.logger.Warn("mcp: invalid resource updated notification", "err", err)
		return
	}
	if updated.URI == "" {
		c.logger.Warn("mcp: resource updated notification requires uri")
		return
	}
	c.mu.RLock()
	subscribed := false
	for subscription := range c.subscribed {
		if resourceUpdateMatchesSubscription(subscription, updated.URI) {
			subscribed = true
			break
		}
	}
	c.mu.RUnlock()
	if !subscribed {
		c.logger.Warn(
			"mcp: resource update received without an active subscription",
			"uri",
			boundedDiagnostic(updated.URI),
		)
		return
	}
	c.emitInvalidation(InvalidationResource, updated.URI)
}

func resourceUpdateMatchesSubscription(subscription, updated string) bool {
	baseURI, baseErr := url.Parse(subscription)
	updatedURI, updatedErr := url.Parse(updated)
	if baseErr != nil || updatedErr != nil ||
		!strings.EqualFold(baseURI.Scheme, updatedURI.Scheme) {
		return false
	}
	if baseURI.Opaque != "" || updatedURI.Opaque != "" {
		return normalizePercentEncoding(baseURI.Opaque) == normalizePercentEncoding(updatedURI.Opaque) &&
			normalizePercentEncoding(baseURI.RawQuery) == normalizePercentEncoding(updatedURI.RawQuery) &&
			normalizePercentEncoding(baseURI.EscapedFragment()) ==
				normalizePercentEncoding(updatedURI.EscapedFragment())
	}
	if !strings.EqualFold(baseURI.Hostname(), updatedURI.Hostname()) ||
		normalizedURIPort(baseURI) != normalizedURIPort(updatedURI) ||
		uriUser(baseURI) != uriUser(updatedURI) {
		return false
	}
	basePath := normalizedURIPath(baseURI)
	updatedPath := normalizedURIPath(updatedURI)
	if basePath == updatedPath &&
		normalizePercentEncoding(baseURI.RawQuery) == normalizePercentEncoding(updatedURI.RawQuery) &&
		normalizePercentEncoding(baseURI.EscapedFragment()) ==
			normalizePercentEncoding(updatedURI.EscapedFragment()) {
		return true
	}
	if baseURI.RawQuery != "" || baseURI.Fragment != "" {
		return false
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return strings.HasPrefix(updatedPath, basePath)
}

func normalizedURIPath(value *url.URL) string {
	escaped := normalizePercentEncoding(value.EscapedPath())
	if escaped == "" {
		return "/"
	}
	return removeDotSegments(escaped)
}

func removeDotSegments(value string) string {
	segments := strings.Split(value, "/")
	output := make([]string, 0, len(segments))
	for _, segment := range segments {
		switch segment {
		case ".":
			continue
		case "..":
			if len(output) > 1 {
				output = output[:len(output)-1]
			}
		default:
			output = append(output, segment)
		}
	}
	result := strings.Join(output, "/")
	if strings.HasSuffix(value, "/.") || strings.HasSuffix(value, "/..") {
		result += "/"
	}
	if result == "" && strings.HasPrefix(value, "/") {
		return "/"
	}
	return result
}

func normalizedURIPort(value *url.URL) string {
	port := value.Port()
	if (strings.EqualFold(value.Scheme, "http") && port == "80") ||
		(strings.EqualFold(value.Scheme, "https") && port == "443") {
		return ""
	}
	return port
}

const (
	hexNibbleBits = 4
	hexNibbleMask = 0x0f
	hexRadixSplit = 10
)

func normalizePercentEncoding(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for index := 0; index < len(value); index++ {
		if value[index] != '%' || index+2 >= len(value) ||
			!isHex(value[index+1]) || !isHex(value[index+2]) {
			normalized.WriteByte(value[index])
			continue
		}
		decoded := fromHex(value[index+1])<<hexNibbleBits | fromHex(value[index+2])
		if isURIUnreserved(decoded) {
			normalized.WriteByte(decoded)
		} else {
			normalized.WriteByte('%')
			normalized.WriteByte(upperHex(decoded >> hexNibbleBits))
			normalized.WriteByte(upperHex(decoded & hexNibbleMask))
		}
		index += 2
	}
	return normalized.String()
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}

func fromHex(value byte) byte {
	if value >= '0' && value <= '9' {
		return value - '0'
	}
	if value >= 'a' && value <= 'f' {
		return value - 'a' + hexRadixSplit
	}
	return value - 'A' + hexRadixSplit
}

func upperHex(value byte) byte {
	if value < hexRadixSplit {
		return '0' + value
	}
	return 'A' + value - hexRadixSplit
}

func isURIUnreserved(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		strings.ContainsRune("-._~", rune(value))
}

func uriUser(value *url.URL) string {
	if value.User == nil {
		return ""
	}
	return value.User.String()
}

func (c *Client) handleLogMessage(params json.RawMessage) {
	c.mu.RLock()
	loggingNegotiated := len(c.server.Capabilities.Logging) > 0
	c.mu.RUnlock()
	if !loggingNegotiated {
		c.logger.Warn("mcp: logging notification contradicts negotiated capability")
		return
	}
	var message LogMessageParams
	if err := json.Unmarshal(params, &message); err != nil {
		c.logger.Warn("mcp: invalid logging notification", "err", err)
		return
	}
	if !validLogLevel(message.Level) || len(message.Data) == 0 {
		c.logger.Warn("mcp: invalid logging notification fields")
		return
	}
	select {
	case c.logMessages <- message:
	default:
		c.logger.Warn("mcp: logging event channel full")
	}
}

func validLogLevel(level string) bool {
	switch level {
	case "debug", "info", "notice", "warning", "error", "critical", "alert", "emergency":
		return true
	default:
		return false
	}
}

func (c *Client) Invalidations() <-chan Invalidation { return c.invalidations }

func (c *Client) LogMessages() <-chan LogMessageParams { return c.logMessages }

func (c *Client) InvalidationGeneration() uint64 { return c.invalidationCount.Load() }

func (c *Client) DiscoveryGeneration(kind InvalidationKind) uint64 {
	switch kind {
	case InvalidationTools:
		return c.toolGeneration.Load()
	case InvalidationResources, InvalidationResource:
		return c.resourceGeneration.Load()
	case InvalidationPrompts:
		return c.promptGeneration.Load()
	default:
		return 0
	}
}

func (c *Client) ServerInfo() InitializeResult {
	c.mu.RLock()
	raw, err := json.Marshal(c.server)
	c.mu.RUnlock()
	if err != nil {
		return InitializeResult{}
	}
	var result InitializeResult
	if json.Unmarshal(raw, &result) != nil {
		return InitializeResult{}
	}
	return result
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.transport.Close()
		if c.invalidations != nil {
			close(c.invalidations)
		}
		if c.logMessages != nil {
			close(c.logMessages)
		}
	})
	return c.closeErr
}
