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

// ClientOption configures the MCP client.
type ClientOption func(*ClientOptions)

// ClientOptions holds client configuration.
type ClientOptions struct {
	Roots []string
}

// WithClientRoots sets the root paths (e.g. workspace folders) to announce to the server.
func WithClientRoots(roots []string) ClientOption {
	return func(o *ClientOptions) {
		o.Roots = roots
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
	o := ClientOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	c := &Client{
		transport: transport,
		opts:      o,
	}
	transport.OnNotification(MethodProgress, c.handleProgress)
	return c
}

func (c *Client) handleProgress(params []byte) {
	var p ProgressParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	if val, ok := c.progressCallbacks.Load(p.ProgressToken); ok {
		if fn, ok := val.(func([]byte)); ok {
			fn(params)
		}
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
		slog.Warn("mcp: failed to send notifications/initialized", "err", err)
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

func (c *Client) toolToProxy(_ context.Context, m *MCPTool) (toolsy.Tool, error) {
	name := m.Name
	description := m.Description
	if description == "" {
		description = m.Title
	}
	if description == "" {
		description = name
	}
	schema := m.InputSchema
	if len(schema) == 0 {
		schema = []byte(`{"type":"object","properties":{}}`)
	}
	transport := c.transport
	handler := func(ctx context.Context, rawArgs []byte, yield func(toolsy.Chunk) error) error {
		progressToken := c.nextProgressToken()
		callParams := ToolsCallParams{
			Name:          name,
			Arguments:     rawArgs,
			ProgressToken: progressToken,
		}
		progressCh := make(chan toolsy.Chunk, 8)
		done := make(chan struct{})
		defer close(done)
		progressFn := func(params []byte) {
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
		c.progressCallbacks.Store(progressToken, progressFn)
		defer c.progressCallbacks.Delete(progressToken)

		resultCh := make(chan callResultWithErr, 1)
		go func() {
			res, reqID, err := transport.Call(ctx, MethodToolsCall, callParams)
			resultCh <- callResultWithErr{res: res, requestID: reqID, err: err}
		}()

		for {
			select {
			case <-ctx.Done():
				// Call may not have sent to resultCh yet; get requestID only if available, then send cancelled only if non-empty.
				var cancelID string
				select {
				case r := <-resultCh:
					cancelID = r.requestID
				default:
				}
				if cancelID != "" {
					_ = transport.Notify(context.Background(), MethodCancelled, CancelledParams{RequestID: json.RawMessage(`"` + cancelID + `"`)})
				}
				return toolsy.ErrStreamAborted
			case r := <-resultCh:
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
				if err := yield(toolsy.Chunk{Event: toolsy.EventResult, Data: chunkData, IsError: chunkIsError}); err != nil {
					if r.requestID != "" {
						_ = transport.Notify(context.Background(), MethodCancelled, CancelledParams{RequestID: json.RawMessage(`"` + r.requestID + `"`)})
					}
					return toolsy.ErrStreamAborted
				}
				return nil
			case ch, ok := <-progressCh:
				if !ok {
					return nil
				}
				if err := yield(ch); err != nil {
					var cancelID string
					select {
					case r := <-resultCh:
						cancelID = r.requestID
					default:
					}
					if cancelID != "" {
						_ = transport.Notify(context.Background(), MethodCancelled, CancelledParams{RequestID: json.RawMessage(`"` + cancelID + `"`)})
					}
					return toolsy.ErrStreamAborted
				}
			}
		}
	}
	return toolsy.NewProxyTool(name, description, schema, handler)
}

type callResultWithErr struct {
	res       []byte
	requestID string
	err       error
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
