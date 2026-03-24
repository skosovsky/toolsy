package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skosovsky/toolsy"
)

const progressChunkBufferSize = 8

// ClientOption configures the MCP client.
type ClientOption func(*ClientOptions)

// ClientOptions holds client configuration.
type ClientOptions struct {
	Roots  []string
	Logger *slog.Logger
}

// WithClientRoots sets the root paths (e.g. workspace folders) to announce to the server.
func WithClientRoots(roots []string) ClientOption {
	return func(o *ClientOptions) {
		o.Roots = roots
	}
}

// WithClientLogger sets the logger for client warnings (e.g. failed notifications). If nil, [slog.Default] is used.
func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(o *ClientOptions) {
		o.Logger = logger
	}
}

// Client is the MCP client that performs the handshake and maps tools/resources/prompts to toolsy.
type Client struct {
	transport Transport
	opts      ClientOptions

	initMu      sync.Mutex
	initialized bool
	initErr     error
	serverCaps  *InitializeResult

	// progressToken -> callback for notifications/progress (one-shot per tools/call).
	progressCallbacks sync.Map
	progressCounter   atomic.Uint64
}

// NewClient creates an MCP client. Call Initialize before using GetTools, GetResourceTool, or GetPrompts.
func NewClient(transport Transport, opts ...ClientOption) *Client {
	o := ClientOptions{
		Roots:  nil,
		Logger: nil,
	}
	for _, opt := range opts {
		opt(&o)
	}
	c := &Client{
		transport:         transport,
		opts:              o,
		initMu:            sync.Mutex{},
		initialized:       false,
		initErr:           nil,
		serverCaps:        nil,
		progressCallbacks: sync.Map{},
		progressCounter:   atomic.Uint64{},
	}
	transport.OnNotification(MethodProgress, c.handleProgress)
	return c
}

func (c *Client) clientLogger() *slog.Logger {
	if c.opts.Logger != nil {
		return c.opts.Logger
	}
	return slog.Default()
}

func (c *Client) handleProgress(params []byte) {
	var p ProgressParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	if val, ok := c.progressCallbacks.Load(p.ProgressToken); ok {
		fn, fnOK := val.(func([]byte))
		if !fnOK {
			return
		}
		fn(params)
	}
}

// Initialize performs the MCP handshake: starts the transport, sends Initialize with client capabilities (roots),
// and stores server capabilities. Must be called once before GetTools, GetResourceTool, or GetPrompts.
func (c *Client) Initialize(ctx context.Context) error {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.initialized {
		if c.initErr != nil {
			return c.initErr
		}
		return nil
	}
	if err := c.transport.Start(ctx); err != nil {
		c.initErr = err
		return err
	}
	roots := c.opts.Roots
	var rootsCap *RootsCapability
	if len(roots) > 0 {
		rootsCap = &RootsCapability{ListChanged: true}
	}
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: ClientCapabilities{
			Roots: rootsCap,
		},
		ClientInfo: ClientInfo{
			Name:    "toolsy-mcp-client",
			Version: "0.1.0",
		},
	}
	// MCP allows roots in params; some servers expect it at top level.
	paramsBytes, _ := json.Marshal(params)
	var paramsWithRoots struct {
		InitializeParams

		Roots []string `json:"roots,omitempty"`
	}
	_ = json.Unmarshal(paramsBytes, &paramsWithRoots)
	paramsWithRoots.Roots = roots
	resultBytes, _, err := c.transport.Call(ctx, MethodInitialize, paramsWithRoots)
	if err != nil {
		c.initErr = err
		return err
	}
	var result InitializeResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		c.initErr = fmt.Errorf("mcp: parse initialize result: %w", err)
		return c.initErr
	}
	c.serverCaps = &result
	c.initialized = true
	// MCP spec requires sending notifications/initialized after successful Initialize;
	// without it the server may stay in "waiting" state and not respond to further requests.
	if err := c.transport.Notify(ctx, MethodInitialized, struct{}{}); err != nil {
		c.clientLogger().WarnContext(ctx, "mcp: failed to send notifications/initialized", "err", err)
	}
	return nil
}

// GetTools returns an iterator over all tools from the server (handles pagination via cursor).
func (c *Client) GetTools(ctx context.Context) iter.Seq2[toolsy.Tool, error] {
	fetch := func(ctx context.Context, cursor string) ([]MCPTool, string, error) {
		if err := c.ensureInitialized(ctx); err != nil {
			return nil, "", err
		}
		params := ToolsListParams{Cursor: cursor}
		res, _, err := c.transport.Call(ctx, MethodToolsList, params)
		if err != nil {
			return nil, "", fmt.Errorf("mcp: list tools: %w", err)
		}
		var result ToolsListResult
		if err := json.Unmarshal(res, &result); err != nil {
			return nil, "", fmt.Errorf("mcp: unmarshal tools: %w", err)
		}
		return result.Tools, result.NextCursor, nil
	}
	return func(yield func(toolsy.Tool, error) bool) {
		for mcpTool, err := range IterateCursor(ctx, fetch) {
			if err != nil {
				yield(nil, err)
				return
			}
			t, toolErr := c.toolToProxy(ctx, &mcpTool)
			if toolErr != nil {
				yield(nil, toolErr)
				return
			}
			if !yield(t, nil) {
				return
			}
		}
	}
}

func (c *Client) ensureInitialized(ctx context.Context) error {
	c.initMu.Lock()
	ok := c.initialized
	err := c.initErr
	c.initMu.Unlock()
	if !ok || err != nil {
		if err != nil {
			return err
		}
		return c.Initialize(ctx)
	}
	return nil
}

func toolDescription(m *MCPTool) string {
	if m.Description != "" {
		return m.Description
	}
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

func defaultToolInputSchemaJSON() []byte {
	return []byte(`{"type":"object","properties":{}}`)
}

func (c *Client) toolToProxy(_ context.Context, m *MCPTool) (toolsy.Tool, error) {
	name := m.Name
	description := toolDescription(m)
	schema := m.InputSchema
	if len(schema) == 0 {
		schema = defaultToolInputSchemaJSON()
	}
	handler := func(ctx context.Context, rawArgs []byte, yield func(toolsy.Chunk) error) error {
		return c.runMCPToolCall(ctx, name, rawArgs, yield)
	}
	return toolsy.NewProxyTool(name, description, schema, handler)
}

type callResultWithErr struct {
	res       []byte
	requestID string
	err       error
}

func (c *Client) notifyCancelledRequest(requestID string) {
	if requestID == "" {
		return
	}
	_ = c.transport.Notify(
		context.Background(),
		MethodCancelled,
		CancelledParams{RequestID: json.RawMessage(`"` + requestID + `"`)},
	)
}

func (c *Client) startToolsCallAsync(
	ctx context.Context,
	transport Transport,
	callParams ToolsCallParams,
	resultCh chan<- callResultWithErr,
) {
	go func() {
		res, reqID, err := transport.Call(ctx, MethodToolsCall, callParams)
		resultCh <- callResultWithErr{res: res, requestID: reqID, err: err}
	}()
}

func readRequestIDIfReady(ch <-chan callResultWithErr) string {
	select {
	case r := <-ch:
		return r.requestID
	default:
		return ""
	}
}

func (c *Client) newToolProgressForwarder(progressCh chan toolsy.Chunk, done chan struct{}) func([]byte) {
	return func(params []byte) {
		select {
		case <-done:
			return
		default:
		}
		var p ProgressParams
		if json.Unmarshal(params, &p) != nil {
			return
		}
		meta := map[string]any{"progressToken": p.ProgressToken}
		if p.Progress >= 0 {
			meta["progress"] = p.Progress
		}
		if p.Total > 0 {
			meta["total"] = p.Total
		}
		if p.ProgressMessage != "" {
			meta["progressMessage"] = p.ProgressMessage
		}
		select {
		case progressCh <- toolsy.Chunk{Event: toolsy.EventProgress, Metadata: meta}:
		case <-done:
		}
	}
}

func (c *Client) runMCPToolCall(
	ctx context.Context,
	name string,
	rawArgs []byte,
	yield func(toolsy.Chunk) error,
) error {
	transport := c.transport
	progressToken := c.nextProgressToken()
	callParams := ToolsCallParams{
		Name:          name,
		Arguments:     rawArgs,
		ProgressToken: progressToken,
	}
	progressCh := make(chan toolsy.Chunk, progressChunkBufferSize)
	done := make(chan struct{})
	defer close(done)

	progressFn := c.newToolProgressForwarder(progressCh, done)
	c.progressCallbacks.Store(progressToken, progressFn)
	defer c.progressCallbacks.Delete(progressToken)

	resultCh := make(chan callResultWithErr, 1)
	c.startToolsCallAsync(ctx, transport, callParams, resultCh)

	for {
		select {
		case <-ctx.Done():
			c.notifyCancelledRequest(readRequestIDIfReady(resultCh))
			return toolsy.ErrStreamAborted
		case r := <-resultCh:
			return c.handleToolCallResult(r, yield)
		case ch, ok := <-progressCh:
			if !ok {
				return nil
			}
			if err := yield(ch); err != nil {
				c.notifyCancelledRequest(readRequestIDIfReady(resultCh))
				return toolsy.ErrStreamAborted
			}
		}
	}
}

func (c *Client) handleToolCallResult(
	r callResultWithErr,
	yield func(toolsy.Chunk) error,
) error {
	if r.err != nil {
		return r.err
	}
	mapped, formatErr := FormatContent(r.res)
	chunkData := r.res
	chunkIsError := false
	if formatErr == nil {
		chunkData = mapped.Data
		chunkIsError = mapped.IsError
	}
	if err := yield(
		toolsy.Chunk{Event: toolsy.EventResult, Data: chunkData, IsError: chunkIsError},
	); err != nil {
		c.notifyCancelledRequest(r.requestID)
		return toolsy.ErrStreamAborted
	}
	return nil
}

func (c *Client) nextProgressToken() string {
	n := c.progressCounter.Add(1)
	return fmt.Sprintf("progress-%d-%d", time.Now().UnixNano(), n)
}

// GetResourceTool returns a single tool that reads resources by URI via resources/read.
func (c *Client) GetResourceTool() (toolsy.Tool, error) {
	schema := []byte(`{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"]}`)
	transport := c.transport
	handler := func(ctx context.Context, argsJSON []byte, yield func(toolsy.Chunk) error) error {
		if err := c.ensureInitialized(ctx); err != nil {
			return err
		}
		var args struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return err
		}
		res, _, err := transport.Call(ctx, MethodResourcesRead, ResourcesReadParams{URI: args.URI})
		if err != nil {
			return fmt.Errorf("mcp: read resource: %w", err)
		}
		mapped, formatErr := FormatContent(res)
		chunkData := res
		chunkIsError := false
		if formatErr == nil {
			chunkData = mapped.Data
			chunkIsError = mapped.IsError
		}
		return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: chunkData, IsError: chunkIsError})
	}
	return toolsy.NewProxyTool("read_mcp_resource", "Reads a resource by URI from the MCP server", schema, handler)
}

// GetPrompts returns an iterator over prompt templates from the server (with cursor pagination).
func (c *Client) GetPrompts(ctx context.Context) iter.Seq2[Prompt, error] {
	fetch := func(ctx context.Context, cursor string) ([]Prompt, string, error) {
		if err := c.ensureInitialized(ctx); err != nil {
			return nil, "", err
		}
		params := PromptsListParams{Cursor: cursor}
		res, _, err := c.transport.Call(ctx, MethodPromptsList, params)
		if err != nil {
			return nil, "", fmt.Errorf("mcp: list prompts: %w", err)
		}
		var result PromptsListResult
		if err := json.Unmarshal(res, &result); err != nil {
			return nil, "", fmt.Errorf("mcp: unmarshal prompts: %w", err)
		}
		return result.Prompts, result.NextCursor, nil
	}
	return IterateCursor(ctx, fetch)
}

// GetPrompt requests a specific prompt with the given arguments and returns the result (Description + Messages).
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptMessageResult, error) {
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
	params := PromptsGetParams{Name: name, Arguments: args}
	resultBytes, _, err := c.transport.Call(ctx, MethodPromptsGet, params)
	if err != nil {
		return nil, fmt.Errorf("mcp: get prompt: %w", err)
	}
	var result PromptsGetResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse prompts/get result: %w", err)
	}
	return &result, nil
}

// Close closes the transport and releases resources.
func (c *Client) Close() error {
	return c.transport.Close()
}
