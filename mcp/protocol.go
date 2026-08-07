// Package mcp provides a strict Model Context Protocol 2026-07-28 client
// bridge for toolsy.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	ProtocolVersion = "2026-07-28"
	JSONRPCVersion  = "2.0"
	jsonNull        = "null"

	ResultTypeComplete      = "complete"
	ResultTypeInputRequired = "input_required"
	progressTokenField      = "progressToken"
	schemaTypeBoolean       = "boolean"
	schemaTypeInteger       = "integer"
	schemaTypeObject        = "object"
	schemaRefKeyword        = "$ref"
	schemaTypeString        = "string"
)

type CacheScope string

const (
	CacheScopePrivate CacheScope = "private"
	CacheScopePublic  CacheScope = "public"
)

const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaLogLevel           = "io.modelcontextprotocol/logLevel"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"
	metaSubscriptionID     = "io.modelcontextprotocol/subscriptionId"
)

var jsonNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

var metaKeyPattern = regexp.MustCompile(
	`^(?:[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*/)?(?:[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)?$`,
)

// Meta preserves additive protocol metadata without interpreting extensions.
type Meta map[string]json.RawMessage

func (m *Meta) UnmarshalJSON(data []byte) error {
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateMetaKeys(fields); err != nil {
		return err
	}
	*m = fields
	return nil
}

func (m Meta) MarshalJSON() ([]byte, error) {
	if err := validateMetaKeys(m); err != nil {
		return nil, err
	}
	for key, raw := range m {
		if err := validateJSONValue(raw); err != nil {
			return nil, fmt.Errorf("mcp: invalid metadata value %q: %w", key, err)
		}
	}
	type plain Meta
	return json.Marshal(plain(m))
}

func validateMetaKeys(meta map[string]json.RawMessage) error {
	for key := range meta {
		if key == "" || strings.HasSuffix(key, "/") || !metaKeyPattern.MatchString(key) {
			return fmt.Errorf("mcp: invalid _meta key %q", key)
		}
	}
	return nil
}

// JSONNumber preserves the exact lexical representation of a JSON number.
type JSONNumber string //nolint:recvcheck // JSON marshaling requires a pointer decoder and value encoder.

func (n *JSONNumber) UnmarshalJSON(data []byte) error {
	if !jsonNumberPattern.Match(data) {
		return errors.New("mcp: value must be a JSON number")
	}
	*n = JSONNumber(data)
	return nil
}
func (n JSONNumber) MarshalJSON() ([]byte, error) {
	if !jsonNumberPattern.MatchString(string(n)) {
		return nil, errors.New("mcp: value must be a JSON number")
	}
	return []byte(n), nil
}
func (n JSONNumber) String() string { return string(n) }

// ProgressToken is an MCP string-or-number progress token.
type ProgressToken struct { //nolint:recvcheck // JSON marshaling requires a pointer decoder and value encoder.
	raw json.RawMessage
}

func NewStringProgressToken(value string) ProgressToken {
	b, _ := json.Marshal(value)
	return ProgressToken{raw: b}
}
func NewIntegerProgressToken(value int64) ProgressToken {
	return ProgressToken{raw: fmt.Appendf(nil, "%d", value)}
}
func (t ProgressToken) IsZero() bool { return len(t.raw) == 0 }
func (t ProgressToken) String() string {
	var s string
	if json.Unmarshal(t.raw, &s) == nil {
		return s
	}
	return string(t.raw)
}
func (t ProgressToken) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte(jsonNull), nil
	}
	return bytes.Clone(t.raw), nil
}
func (t *ProgressToken) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte(jsonNull)) {
		return errors.New("mcp: progress token must not be null")
	}
	var s string
	if json.Unmarshal(data, &s) == nil {
		t.raw = bytes.Clone(data)
		return nil
	}
	var n JSONNumber
	if json.Unmarshal(data, &n) == nil {
		t.raw = bytes.Clone(data)
		return nil
	}
	return errors.New("mcp: progress token must be a string or number")
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type JSONRPCError struct {
	Code    JSONNumber      `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Icon struct {
	Src      string   `json:"src"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
	Extra    Meta     `json:"-"`
}
type Implementation struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Icons       []Icon `json:"icons,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
	Extra       Meta   `json:"-"`
}

// ClientCapabilities describes the current protocol's client declarations.
// Deprecated declarations remain inert wire data; they do not restore legacy handlers.
type ClientCapabilities struct {
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
	Roots        *RootsCapability           `json:"roots,omitempty"`
	Sampling     *SamplingCapability        `json:"sampling,omitempty"`
	Elicitation  *ElicitationCapability     `json:"elicitation,omitempty"`
	Extensions   map[string]json.RawMessage `json:"extensions,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
}

// RootsCapability is an inert declaration for the deprecated roots feature.
type RootsCapability struct {
	Extra Meta `json:"-"`
}

// SamplingCapability is an inert declaration for the deprecated sampling feature.
type SamplingCapability struct {
	Context json.RawMessage `json:"context,omitempty"`
	Tools   json.RawMessage `json:"tools,omitempty"`
	Extra   Meta            `json:"-"`
}

// ElicitationCapability declares supported elicitation modes without installing handlers.
type ElicitationCapability struct {
	Form  json.RawMessage `json:"form,omitempty"`
	URL   json.RawMessage `json:"url,omitempty"`
	Extra Meta            `json:"-"`
}
type ServerCapabilities struct {
	Tools      *ToolsCapability           `json:"tools,omitempty"`
	Resources  *ResourcesCapability       `json:"resources,omitempty"`
	Prompts    *PromptsCapability         `json:"prompts,omitempty"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	Extra      map[string]json.RawMessage `json:"-"`
}
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
	Extra       Meta `json:"-"`
}
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
	Extra       Meta `json:"-"`
}
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
	Extra       Meta `json:"-"`
}

type RequestMeta struct {
	ProtocolVersion    string             `json:"io.modelcontextprotocol/protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"io.modelcontextprotocol/clientCapabilities"`
	ClientInfo         *Implementation    `json:"io.modelcontextprotocol/clientInfo,omitempty"`
	LogLevel           string             `json:"io.modelcontextprotocol/logLevel,omitempty"`
	ProgressToken      ProgressToken      `json:"progressToken,omitzero"`
	Extra              Meta               `json:"-"`
}
type ResultMeta struct {
	ServerInfo *Implementation `json:"io.modelcontextprotocol/serverInfo,omitempty"`
	Extra      Meta            `json:"-"`
}
type RequestParams struct {
	Meta *RequestMeta `json:"_meta"`
}
type NotificationParams struct {
	Meta  Meta `json:"_meta,omitempty"`
	Extra Meta `json:"-"`
}

type DiscoverParams = RequestParams

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type DiscoverResult struct {
	CacheInfo

	ResultType        string             `json:"resultType"`
	SupportedVersions []string           `json:"supportedVersions"`
	Capabilities      ServerCapabilities `json:"capabilities"`
	Instructions      string             `json:"instructions,omitempty"`
	Meta              ResultMeta         `json:"_meta,omitzero"`
	Extra             Meta               `json:"-"`
}

type CacheInfo struct {
	TTLMS      JSONNumber `json:"ttlMs"`
	CacheScope CacheScope `json:"cacheScope"`
}
type CompleteResult struct {
	ResultType string     `json:"resultType"`
	Meta       ResultMeta `json:"_meta,omitzero"`
	Extra      Meta       `json:"-"`
}
type EmptyResult = CompleteResult
type InputRequiredResult struct {
	ResultType    string          `json:"resultType"`
	InputRequests json.RawMessage `json:"inputRequests,omitempty"`
	RequestState  json.RawMessage `json:"requestState,omitempty"`
	Meta          ResultMeta      `json:"_meta,omitzero"`
	Extra         Meta            `json:"-"`
}

type CursorParams struct {
	Cursor string       `json:"cursor,omitempty"`
	Meta   *RequestMeta `json:"_meta"`
}
type ToolsListParams = CursorParams

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type ToolsListResult struct {
	CacheInfo

	ResultType string     `json:"resultType"`
	Tools      []MCPTool  `json:"tools"`
	NextCursor string     `json:"nextCursor,omitempty"`
	Meta       ResultMeta `json:"_meta,omitzero"`
	Extra      Meta       `json:"-"`
}
type MCPTool struct { //nolint:revive // MCPTool distinguishes wire descriptors from toolsy.Tool.
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  json.RawMessage  `json:"inputSchema"`
	OutputSchema json.RawMessage  `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
	Meta         Meta             `json:"_meta,omitempty"`
	Extra        Meta             `json:"-"`
}
type ToolsCallParams struct {
	Name           string          `json:"name"`
	Arguments      json.RawMessage `json:"arguments,omitempty"`
	InputResponses json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   json.RawMessage `json:"requestState,omitempty"`
	Meta           *RequestMeta    `json:"_meta"`
}
type CallToolResult struct {
	ResultType        string          `json:"resultType"`
	Content           []ContentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
	Meta              ResultMeta      `json:"_meta,omitzero"`
	Extra             Meta            `json:"-"`
}

type Annotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
	Extra        Meta     `json:"-"`
}
type ResourceContents struct {
	URI         string  `json:"uri"`
	MIMEType    string  `json:"mimeType,omitempty"`
	Text        *string `json:"text,omitempty"`
	Blob        *string `json:"blob,omitempty"`
	Meta        Meta    `json:"_meta,omitempty"`
	Extra       Meta    `json:"-"`
	textPresent bool
	blobPresent bool
}

//nolint:recvcheck // Value marshaling and pointer unmarshaling implement encoding/json contracts.
type ContentBlock struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	Data        string            `json:"data,omitempty"`
	MIMEType    string            `json:"mimeType,omitempty"`
	URI         string            `json:"uri,omitempty"`
	Name        string            `json:"name,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Size        *JSONNumber       `json:"size,omitempty"`
	Icons       []Icon            `json:"icons,omitempty"`
	Resource    *ResourceContents `json:"resource,omitempty"`
	Annotations *Annotations      `json:"annotations,omitempty"`
	Meta        Meta              `json:"_meta,omitempty"`
	wireFields  map[string]bool
}

type ResourcesListParams = CursorParams

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type ResourcesListResult struct {
	CacheInfo

	ResultType string     `json:"resultType"`
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
	Meta       ResultMeta `json:"_meta,omitzero"`
	Extra      Meta       `json:"-"`
}
type Resource struct {
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	MIMEType    string       `json:"mimeType,omitempty"`
	Size        *JSONNumber  `json:"size,omitempty"`
	Icons       []Icon       `json:"icons,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
	Extra       Meta         `json:"-"`
}
type ResourceTemplatesListParams = CursorParams

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type ResourceTemplatesListResult struct {
	CacheInfo

	ResultType        string             `json:"resultType"`
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	NextCursor        string             `json:"nextCursor,omitempty"`
	Meta              ResultMeta         `json:"_meta,omitzero"`
	Extra             Meta               `json:"-"`
}
type ResourceTemplate struct {
	URITemplate string       `json:"uriTemplate"`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	MIMEType    string       `json:"mimeType,omitempty"`
	Icons       []Icon       `json:"icons,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
	Extra       Meta         `json:"-"`
}
type ResourcesReadParams struct {
	URI            string          `json:"uri"`
	InputResponses json.RawMessage `json:"inputResponses,omitempty"`
	RequestState   json.RawMessage `json:"requestState,omitempty"`
	Meta           *RequestMeta    `json:"_meta"`
}

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type ResourcesReadResult struct {
	CacheInfo

	ResultType string             `json:"resultType"`
	Contents   []ResourceContents `json:"contents"`
	Meta       ResultMeta         `json:"_meta,omitzero"`
	Extra      Meta               `json:"-"`
}

type PromptsListParams = CursorParams

//nolint:recvcheck // JSON decoding mutates while snapshot identity is value-semantic.
type PromptsListResult struct {
	CacheInfo

	ResultType string     `json:"resultType"`
	Prompts    []Prompt   `json:"prompts"`
	NextCursor string     `json:"nextCursor,omitempty"`
	Meta       ResultMeta `json:"_meta,omitzero"`
	Extra      Meta       `json:"-"`
}
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Icons       []Icon           `json:"icons,omitempty"`
	Meta        Meta             `json:"_meta,omitempty"`
	Extra       Meta             `json:"-"`
}
type PromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Extra       Meta   `json:"-"`
}
type PromptsGetParams struct {
	Name           string            `json:"name"`
	Arguments      map[string]string `json:"arguments,omitempty"`
	InputResponses json.RawMessage   `json:"inputResponses,omitempty"`
	RequestState   json.RawMessage   `json:"requestState,omitempty"`
	Meta           *RequestMeta      `json:"_meta"`
}
type PromptsGetResult struct {
	ResultType  string          `json:"resultType"`
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
	Meta        ResultMeta      `json:"_meta,omitzero"`
	Extra       Meta            `json:"-"`
}
type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
	Extra   Meta         `json:"-"`
}

type SubscriptionFilter struct {
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
	Extra                 Meta     `json:"-"`
}
type SubscriptionsListenParams struct {
	Notifications SubscriptionFilter `json:"notifications"`
	Meta          *RequestMeta       `json:"_meta"`
	Extra         Meta               `json:"-"`
}
type SubscriptionsListenResult struct {
	ResultType string                 `json:"resultType"`
	Meta       SubscriptionResultMeta `json:"_meta"`
	Extra      Meta                   `json:"-"`
}
type SubscriptionResultMeta struct {
	SubscriptionID json.RawMessage `json:"io.modelcontextprotocol/subscriptionId"`
	ServerInfo     *Implementation `json:"io.modelcontextprotocol/serverInfo,omitempty"`
	Extra          Meta            `json:"-"`
}
type SubscriptionsAcknowledgedParams struct {
	Notifications SubscriptionFilter `json:"notifications"`
	Meta          Meta               `json:"_meta,omitempty"`
	Extra         Meta               `json:"-"`
}
type ResourceUpdatedParams struct {
	URI   string `json:"uri"`
	Meta  Meta   `json:"_meta,omitempty"`
	Extra Meta   `json:"-"`
}
type LogMessageParams struct {
	Level  string          `json:"level"`
	Logger string          `json:"logger,omitempty"`
	Data   json.RawMessage `json:"data"`
	Meta   Meta            `json:"_meta,omitempty"`
	Extra  Meta            `json:"-"`
}
type ProgressParams struct {
	ProgressToken ProgressToken `json:"progressToken"`
	Progress      float64       `json:"progress"`
	Total         *float64      `json:"total,omitempty"`
	Message       string        `json:"message,omitempty"`
	Meta          Meta          `json:"_meta,omitempty"`
	Extra         Meta          `json:"-"`
}
type CancelledParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    string          `json:"reason,omitempty"`
	Meta      Meta            `json:"_meta,omitempty"`
	Extra     Meta            `json:"-"`
}

const (
	MethodServerDiscover            = "server/discover"
	MethodToolsList                 = "tools/list"
	MethodToolsCall                 = "tools/call"
	MethodToolsListChanged          = "notifications/tools/list_changed"
	MethodResourcesList             = "resources/list"
	MethodResourceTemplatesList     = "resources/templates/list"
	MethodResourcesRead             = "resources/read"
	MethodResourcesListChanged      = "notifications/resources/list_changed"
	MethodResourceUpdated           = "notifications/resources/updated"
	MethodPromptsList               = "prompts/list"
	MethodPromptsGet                = "prompts/get"
	MethodPromptsListChanged        = "notifications/prompts/list_changed"
	MethodSubscriptionsListen       = "subscriptions/listen"
	MethodSubscriptionsAcknowledged = "notifications/subscriptions/acknowledged"
	MethodProgress                  = "notifications/progress"
	MethodCancelled                 = "notifications/cancelled"
	MethodLogMessage                = "notifications/message"
)

const (
	JSONRPCParseError                      JSONNumber = "-32700"
	JSONRPCInvalidRequest                  JSONNumber = "-32600"
	JSONRPCMethodNotFound                  JSONNumber = "-32601"
	JSONRPCInvalidParams                   JSONNumber = "-32602"
	JSONRPCInternalError                   JSONNumber = "-32603"
	JSONRPCHeaderMismatch                  JSONNumber = "-32020"
	JSONRPCMissingRequiredClientCapability JSONNumber = "-32021"
	JSONRPCUnsupportedProtocolVersion      JSONNumber = "-32022"
)

func mergeExtra(base any, extra Meta, reserved ...string) ([]byte, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	fields, err := decodeObjectFields(raw)
	if err != nil {
		return nil, err
	}
	blocked := make(map[string]struct{}, len(reserved))
	for _, k := range reserved {
		blocked[k] = struct{}{}
	}
	for k, v := range extra {
		if _, ok := blocked[k]; ok {
			return nil, fmt.Errorf("mcp: extension field %q collides with reserved field", k)
		}
		if err := validateJSONValue(v); err != nil {
			return nil, fmt.Errorf("mcp: invalid extension field %q: %w", k, err)
		}
		fields[k] = bytes.Clone(v)
	}
	return json.Marshal(fields)
}
