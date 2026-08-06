// Package mcp provides a strict Model Context Protocol 2025-11-25 client bridge
// for toolsy. It supports stdio and Streamable HTTP transports.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
)

const (
	ProtocolVersion           = "2025-11-25"
	JSONRPCVersion            = "2.0"
	jsonNull                  = "null"
	rpcMethodNotFoundMessage  = "Method not found"
	contentTypeText           = "text"
	contentTypeImage          = "image"
	contentTypeAudio          = "audio"
	contentTypeResourceLink   = "resource_link"
	contentTypeResource       = "resource"
	audienceUser              = "user"
	audienceAssistant         = "assistant"
	applicationOctetStream    = "application/octet-stream"
	mimeTypeField             = "mimeType"
	nameField                 = "name"
	titleField                = "title"
	descriptionField          = "description"
	annotationsField          = "annotations"
	metaField                 = "_meta"
	dataField                 = "data"
	serverInfoField           = "serverInfo"
	capabilitiesField         = "capabilities"
	clientInfoField           = "clientInfo"
	taskSupportOptional       = "optional"
	taskSupportForbidden      = "forbidden"
	taskSupportRequired       = "required"
	capabilityToolsField      = "tools"
	capabilityResourcesField  = "resources"
	capabilityPromptsField    = "prompts"
	capabilityLoggingField    = "logging"
	capabilityCompleteField   = "completions"
	capabilityTasksField      = "tasks"
	capabilityExperimentField = "experimental"
)

var jsonNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

var metaKeyPattern = regexp.MustCompile(
	`^(?:[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*/)?` +
		`(?:[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)?$`,
)

// Meta preserves additive MCP metadata fields.
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
	type plain Meta
	return json.Marshal(plain(m))
}

func validateMetaKeys(meta map[string]json.RawMessage) error {
	for key := range meta {
		if !metaKeyPattern.MatchString(key) {
			return errors.New("mcp: invalid _meta key format")
		}
	}
	return nil
}

// ProgressToken is an MCP string-or-number progress token.
type ProgressToken struct { //nolint:recvcheck // json.Marshaler requires value semantics; UnmarshalJSON must mutate.
	raw json.RawMessage
}

// JSONNumber preserves the exact lexical representation of a JSON number.
type JSONNumber string //nolint:recvcheck // json.Marshaler uses value semantics; UnmarshalJSON must mutate.

func (n *JSONNumber) UnmarshalJSON(data []byte) error {
	value := string(data)
	if !jsonNumberPattern.MatchString(value) {
		return errors.New("mcp: value must be a JSON number")
	}
	*n = JSONNumber(value)
	return nil
}

func (n JSONNumber) MarshalJSON() ([]byte, error) {
	value := string(n)
	if !jsonNumberPattern.MatchString(value) {
		return nil, errors.New("mcp: value must be a JSON number")
	}
	return []byte(value), nil
}

func (n JSONNumber) String() string { return string(n) }

func NewStringProgressToken(value string) ProgressToken {
	b, _ := json.Marshal(value)
	return ProgressToken{raw: b}
}

func NewIntegerProgressToken(value int64) ProgressToken {
	return ProgressToken{raw: fmt.Appendf(nil, "%d", value)}
}

func (t ProgressToken) IsZero() bool { return len(t.raw) == 0 }

func (t ProgressToken) String() string {
	if len(t.raw) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(t.raw, &value) == nil {
		return value
	}
	return string(t.raw)
}

func (t ProgressToken) MarshalJSON() ([]byte, error) {
	if len(t.raw) == 0 {
		return []byte(jsonNull), nil
	}
	return bytes.Clone(t.raw), nil
}

func (t *ProgressToken) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte(jsonNull)) {
		return errors.New("mcp: progress token must be a string or number")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	switch v := value.(type) {
	case string:
	case json.Number:
		if !jsonNumberPattern.MatchString(v.String()) {
			return errors.New("mcp: invalid numeric progress token")
		}
	default:
		return errors.New("mcp: progress token must be a string or number")
	}
	t.raw = bytes.Clone(data)
	return nil
}

type RequestMeta struct {
	ProgressToken ProgressToken `json:"progressToken,omitzero"`
	Extra         Meta          `json:"-"`
}

func (m *RequestMeta) UnmarshalJSON(data []byte) error {
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	var decoded RequestMeta
	if raw, ok := fields["progressToken"]; ok {
		if err := json.Unmarshal(raw, &decoded.ProgressToken); err != nil {
			return err
		}
		delete(fields, "progressToken")
	}
	if len(fields) > 0 {
		if err := validateMetaKeys(fields); err != nil {
			return err
		}
		decoded.Extra = fields
	}
	*m = decoded
	return nil
}

func (m RequestMeta) MarshalJSON() ([]byte, error) {
	if err := validateMetaKeys(m.Extra); err != nil {
		return nil, err
	}
	if _, reserved := m.Extra["progressToken"]; reserved {
		return nil, errors.New("mcp: RequestMeta.Extra must not contain reserved progressToken")
	}
	fields := make(map[string]json.RawMessage, len(m.Extra)+1)
	maps.Copy(fields, m.Extra)
	if !m.ProgressToken.IsZero() {
		raw, err := json.Marshal(m.ProgressToken)
		if err != nil {
			return nil, err
		}
		fields["progressToken"] = raw
	}
	return json.Marshal(fields)
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    JSONNumber      `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// EmptyResult is an MCP Result object without method-specific fields.
// Extra preserves additive top-level Result fields.
type EmptyResult struct {
	Meta  Meta `json:"_meta,omitempty"`
	Extra Meta `json:"-"`
}

func (e JSONRPCError) MarshalJSON() ([]byte, error) {
	type wireError JSONRPCError
	return marshalStrict[JSONRPCError](wireError(e))
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type NotificationParams struct {
	Meta Meta `json:"_meta,omitempty"`
}

type RequestParams struct {
	Meta *RequestMeta `json:"_meta,omitempty"`
}

type Icon struct {
	Src      string   `json:"src"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
	Theme    string   `json:"theme,omitempty"`
}

type Implementation struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Icons       []Icon `json:"icons,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
	Meta            *RequestMeta       `json:"_meta,omitempty"`
}

type ClientCapabilities struct {
	Roots *RootsCapability `json:"roots,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
	Meta            Meta               `json:"_meta,omitempty"`
	Extra           Meta               `json:"-"`
}

type ServerCapabilities struct {
	Tools        *ToolsCapability           `json:"tools,omitempty"`
	Resources    *ResourcesCapability       `json:"resources,omitempty"`
	Prompts      *PromptsCapability         `json:"prompts,omitempty"`
	Logging      json.RawMessage            `json:"logging,omitempty"`
	Completions  json.RawMessage            `json:"completions,omitempty"`
	Tasks        json.RawMessage            `json:"tasks,omitempty"`
	Experimental json.RawMessage            `json:"experimental,omitempty"`
	Extra        map[string]json.RawMessage `json:"-"`
}

func (c *ServerCapabilities) UnmarshalJSON(data []byte) error {
	type plain ServerCapabilities
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	knownCapabilities := []string{
		capabilityToolsField, capabilityResourcesField, capabilityPromptsField,
		capabilityLoggingField, capabilityCompleteField, capabilityTasksField,
		capabilityExperimentField,
	}
	if err := validateKnownServerCapabilities(fields); err != nil {
		return err
	}
	for _, known := range knownCapabilities {
		delete(fields, known)
	}
	*c = ServerCapabilities(decoded)
	if len(fields) > 0 {
		c.Extra = fields
	}
	return nil
}

func validateKnownServerCapabilities(fields map[string]json.RawMessage) error {
	for _, name := range []string{
		capabilityToolsField, capabilityResourcesField, capabilityPromptsField,
		capabilityLoggingField, capabilityCompleteField, capabilityTasksField,
		capabilityExperimentField,
	} {
		if err := validateOptionalObject(fields, name); err != nil {
			return fmt.Errorf("mcp: invalid %s capability: %w", name, err)
		}
	}
	if raw, ok := fields[capabilityExperimentField]; ok {
		experimental, err := decodeObjectFields(raw)
		if err != nil {
			return fmt.Errorf("mcp: invalid experimental capability: %w", err)
		}
		for name, value := range experimental {
			if _, err := decodeObjectFields(value); err != nil {
				return fmt.Errorf("mcp: experimental capability %q must be a non-null object: %w", name, err)
			}
		}
	}
	if raw, ok := fields[capabilityTasksField]; ok {
		if err := validateServerTasksCapability(raw); err != nil {
			return fmt.Errorf("mcp: invalid tasks capability: %w", err)
		}
	}
	return nil
}

func validateServerTasksCapability(raw json.RawMessage) error {
	tasks, err := decodeObjectFields(raw)
	if err != nil {
		return err
	}
	for _, name := range []string{"cancel", "list"} {
		if validationErr := validateOptionalObject(tasks, name); validationErr != nil {
			return validationErr
		}
	}
	requestsRaw, ok := tasks["requests"]
	if !ok {
		return nil
	}
	requests, err := decodeObjectFields(requestsRaw)
	if err != nil {
		return fmt.Errorf("field %q must be a non-null object: %w", "requests", err)
	}
	toolsRaw, ok := requests["tools"]
	if !ok {
		return nil
	}
	tools, err := decodeObjectFields(toolsRaw)
	if err != nil {
		return fmt.Errorf("field %q must be a non-null object: %w", "requests.tools", err)
	}
	if err := validateOptionalObject(tools, "call"); err != nil {
		return fmt.Errorf("field %q must be a non-null object: %w", "requests.tools.call", err)
	}
	return nil
}

func (c ServerCapabilities) MarshalJSON() ([]byte, error) {
	type plain ServerCapabilities
	base, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	delete(fields, "Extra")
	reserved := map[string]struct{}{
		capabilityToolsField: {}, capabilityResourcesField: {}, capabilityPromptsField: {},
		capabilityLoggingField: {}, capabilityCompleteField: {}, capabilityTasksField: {},
		capabilityExperimentField: {},
	}
	for name, value := range c.Extra {
		if _, collision := reserved[name]; collision {
			return nil, fmt.Errorf("mcp: capability extension %q collides with a reserved field", name)
		}
		fields[name] = value
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[ServerCapabilities](encoded)
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type CursorParams struct {
	Cursor string       `json:"cursor,omitempty"`
	Meta   *RequestMeta `json:"_meta,omitempty"`
}

type ToolsListParams = CursorParams

type ToolsListResult struct {
	Tools      []MCPTool `json:"tools"`
	NextCursor string    `json:"nextCursor,omitempty"`
	Meta       Meta      `json:"_meta,omitempty"`
	Extra      Meta      `json:"-"`
}

type ToolExecution struct {
	TaskSupport string `json:"taskSupport,omitempty"`
}

type MCPTool struct { //nolint:revive // MCPTool distinguishes the wire descriptor from toolsy.Tool.
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  json.RawMessage  `json:"inputSchema"`
	OutputSchema json.RawMessage  `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
	Execution    *ToolExecution   `json:"execution,omitempty"`
	Meta         Meta             `json:"_meta,omitempty"`
}

type ToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      *RequestMeta    `json:"_meta,omitempty"`
}

type Annotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
}

// ResourceContents represents either TextResourceContents or BlobResourceContents.
type ResourceContents struct {
	URI         string  `json:"uri"`
	MIMEType    string  `json:"mimeType,omitempty"`
	Text        *string `json:"text,omitempty"`
	Blob        *string `json:"blob,omitempty"`
	Meta        Meta    `json:"_meta,omitempty"`
	textPresent bool
	blobPresent bool
}

// ContentBlock is the lossless tagged union used by tool results and prompt messages.
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

type CallToolResult struct {
	Content           []ContentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
	Meta              Meta            `json:"_meta,omitempty"`
	Extra             Meta            `json:"-"`
}

type ResourcesReadParams struct {
	URI  string       `json:"uri"`
	Meta *RequestMeta `json:"_meta,omitempty"`
}

type ResourcesReadResult struct {
	Contents []ResourceContents `json:"contents"`
	Meta     Meta               `json:"_meta,omitempty"`
	Extra    Meta               `json:"-"`
}

type PromptsListParams = CursorParams

type PromptsListResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
	Meta       Meta     `json:"_meta,omitempty"`
	Extra      Meta     `json:"-"`
}

type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Icons       []Icon           `json:"icons,omitempty"`
	Meta        Meta             `json:"_meta,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type PromptsGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
	Meta      *RequestMeta      `json:"_meta,omitempty"`
}

type PromptsGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
	Meta        Meta            `json:"_meta,omitempty"`
	Extra       Meta            `json:"-"`
}

type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

type ProgressParams struct {
	ProgressToken ProgressToken `json:"progressToken"`
	Progress      float64       `json:"progress"`
	Total         *float64      `json:"total,omitempty"`
	Message       string        `json:"message,omitempty"`
	Meta          Meta          `json:"_meta,omitempty"`
}

type CancelledParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    string          `json:"reason,omitempty"`
	Meta      Meta            `json:"_meta,omitempty"`
}

type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
	Meta Meta   `json:"_meta,omitempty"`
}

type RootsListResult struct {
	Roots []Root `json:"roots"`
	Meta  Meta   `json:"_meta,omitempty"`
	Extra Meta   `json:"-"`
}

type ResourceUpdatedParams struct {
	URI  string `json:"uri"`
	Meta Meta   `json:"_meta,omitempty"`
}

type LogMessageParams struct {
	Level  string          `json:"level"`
	Logger string          `json:"logger,omitempty"`
	Data   json.RawMessage `json:"data"`
	Meta   Meta            `json:"_meta,omitempty"`
}

const (
	MethodInitialize           = "initialize"
	MethodInitialized          = "notifications/initialized"
	MethodPing                 = "ping"
	MethodRootsList            = "roots/list"
	MethodRootsListChanged     = "notifications/roots/list_changed"
	MethodToolsList            = "tools/list"
	MethodToolsCall            = "tools/call"
	MethodToolsListChanged     = "notifications/tools/list_changed"
	MethodResourcesRead        = "resources/read"
	MethodResourcesSubscribe   = "resources/subscribe"
	MethodResourcesListChanged = "notifications/resources/list_changed"
	MethodResourceUpdated      = "notifications/resources/updated"
	MethodPromptsList          = "prompts/list"
	MethodPromptsGet           = "prompts/get"
	MethodPromptsListChanged   = "notifications/prompts/list_changed"
	MethodProgress             = "notifications/progress"
	MethodCancelled            = "notifications/cancelled"
	MethodLogMessage           = "notifications/message"
)

const (
	JSONRPCParseError       JSONNumber = "-32700"
	JSONRPCInvalidRequest   JSONNumber = "-32600"
	JSONRPCMethodNotFound   JSONNumber = "-32601"
	JSONRPCInvalidParams    JSONNumber = "-32602"
	JSONRPCInternalError    JSONNumber = "-32603"
	JSONRPCServerOverloaded JSONNumber = "-32000"
)
