// Package mcp provides a Model Context Protocol (MCP) client that bridges
// MCP servers to toolsy's Tool/Registry interface. It supports stdio and SSE transports.
package mcp

import (
	"encoding/json"
)

// JSON-RPC 2.0 message types.

// Request is a JSON-RPC 2.0 request (id present, expects response).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (no id, no response expected).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP Initialize.

// InitializeParams is sent by the client in the Initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities declares what the client supports (e.g. roots for file access).
type ClientCapabilities struct {
	Roots *RootsCapability `json:"roots,omitempty"`
}

// RootsCapability indicates the client supports roots (folder paths).
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientInfo identifies the client implementation.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server response to Initialize.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ServerCapabilities describes what the server provides.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *struct{}            `json:"logging,omitempty"`
}

// ToolsCapability indicates the server supports tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates the server supports resources.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates the server supports prompts.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the server implementation.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tools: tools/list, tools/call.

// ToolsListParams is the optional params for tools/list (pagination).
type ToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ToolsListResult is the result of tools/list.
type ToolsListResult struct {
	Tools      []MCPTool `json:"tools"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

// MCPTool is a tool descriptor from tools/list. Title is used by UI (e.g. Claude Desktop) and as fallback for description.
// Name is intentionally MCPTool (not Tool) to avoid stutter and conflict with toolsy.Tool.
//
//revive:disable-next-line:exported
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Title       string          `json:"title,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsCallParams is the params for tools/call.
type ToolsCallParams struct {
	Name          string          `json:"name"`
	Arguments     json.RawMessage `json:"arguments"`
	ProgressToken string          `json:"progressToken,omitempty"`
}

// ToolsCallResult is the result of tools/call.
type ToolsCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem is a single content piece (text or base64).
type ContentItem struct {
	Type      string `json:"type"` // "text" or "image"
	Text      string `json:"text,omitempty"`
	Base64    string `json:"base64,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

// Resources: resources/read.

// ResourcesReadParams is the params for resources/read.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// ResourcesReadResult is the result of resources/read.
type ResourcesReadResult struct {
	Contents []ContentItem `json:"contents"`
}

// Prompts: prompts/list, prompts/get.

// PromptsListParams is the optional params for prompts/list (pagination).
type PromptsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// PromptsListResult is the result of prompts/list.
type PromptsListResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// Prompt is a prompt template descriptor from the server.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes a prompt template argument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptsGetParams is the params for prompts/get.
type PromptsGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptsGetResult is the result of prompts/get (description + messages).
type PromptsGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessageResult is the result of GetPrompt; it contains Description and Messages (alias for PromptsGetResult).
type PromptMessageResult = PromptsGetResult

// PromptMessage is a single message in a prompt result (role + content).
type PromptMessage struct {
	Role    string          `json:"role"` // "user" or "assistant"
	Content *ContentMessage `json:"content,omitempty"`
}

// ContentMessage holds text or parts for a message.
type ContentMessage struct {
	Type  string        `json:"type,omitempty"` // "text"
	Text  string        `json:"text,omitempty"`
	Parts []ContentItem `json:"parts,omitempty"`
}

// Notifications: progress and cancelled.

// ProgressParams is the params for notifications/progress.
type ProgressParams struct {
	ProgressToken   string `json:"progressToken"`
	Progress        int    `json:"progress,omitempty"` // 0-100
	Total           int    `json:"total,omitempty"`
	ProgressMessage string `json:"progressMessage,omitempty"`
}

// CancelledParams is the params for notifications/cancelled.
type CancelledParams struct {
	RequestID json.RawMessage `json:"requestId"`
}

const (
	// JSONRPCVersion is the JSON-RPC version string.
	JSONRPCVersion = "2.0"
	// MethodInitialize is the MCP Initialize method.
	MethodInitialize = "initialize"
	// MethodInitialized is the notification sent after successful init.
	MethodInitialized = "notifications/initialized"
	// MethodToolsList is tools/list.
	MethodToolsList = "tools/list"
	// MethodToolsCall is tools/call.
	MethodToolsCall = "tools/call"
	// MethodResourcesRead is resources/read.
	MethodResourcesRead = "resources/read"
	// MethodPromptsList is prompts/list.
	MethodPromptsList = "prompts/list"
	// MethodPromptsGet is prompts/get.
	MethodPromptsGet = "prompts/get"
	// MethodProgress is notifications/progress.
	MethodProgress = "notifications/progress"
	// MethodCancelled is notifications/cancelled.
	MethodCancelled = "notifications/cancelled"
)
