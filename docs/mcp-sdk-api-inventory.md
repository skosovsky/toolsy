# Baseline MCP API and test inventory

Baseline: `4f7eae6cc2393da259cf30a5535bef12c93d6e2c`. Recover historical source with `git show 4f7eae6:mcp/<file>`. These are baseline references, not links to surviving files. This inventory records exported declarations; it does not claim preservation. Capability disposition and target owners are in [the design matrix](mcp-sdk-integration-design.md#capability-coverage-matrix).

## client.go

- `type ClientOption func(*ClientOptions)` — baseline `mcp/client.go:52`
- `type ClientOptions struct` — baseline `mcp/client.go:54`
- `func WithClientLogger(logger *slog.Logger) ClientOption` — baseline `mcp/client.go:62`
- `func WithClientInfo(info Implementation) ClientOption` — baseline `mcp/client.go:66`
- `func WithPaginationLimits(limits PaginationLimits) ClientOption` — baseline `mcp/client.go:71`
- `type InvalidationKind string` — baseline `mcp/client.go:75`
- `type Invalidation struct` — baseline `mcp/client.go:85`
- `type InputRequiredError struct` — baseline `mcp/client.go:94`
- `func (e *InputRequiredError) Error() string` — baseline `mcp/client.go:99`
- `type Subscription struct` — baseline `mcp/client.go:104`
- `func (s *Subscription) Done() <-chan struct` — baseline `mcp/client.go:112`
- `func (s *Subscription) Err() error` — baseline `mcp/client.go:113`
- `func (s *Subscription) Close()` — baseline `mcp/client.go:114`
- `type Client struct` — baseline `mcp/client.go:141`
- `func Connect(ctx context.Context, transport Transport, opts ...ClientOption) (*Client, error)` — baseline `mcp/client.go:168`
- `func (c *Client) ListToolsPage(ctx context.Context, cursor string) (ToolsListResult, error)` — baseline `mcp/client.go:887`
- `func (c *Client) DiscoverTools(ctx context.Context) (ToolDiscovery, error)` — baseline `mcp/client.go:898`
- `func (c *Client) Discover(ctx context.Context) iter.Seq2[toolsy.Tool, error]` — baseline `mcp/client.go:904`
- `func (c *Client) CallTool(ctx context.Context, params ToolsCallParams) (*CallToolResult, *InputRequiredResult, error)` — baseline `mcp/client.go:1192`
- `func (c *Client) GetResourceTool() (toolsy.Tool, error)` — baseline `mcp/client.go:1391`
- `func (c *Client) ReadResource(ctx context.Context, uri string) (ResourcesReadResult, error)` — baseline `mcp/client.go:1438`
- `func (c *Client) ReadResourceRound(` — baseline `mcp/client.go:1452`
- `func (c *Client) ListResources(ctx context.Context, cursor string) (ResourcesListResult, error)` — baseline `mcp/client.go:1524`
- `func (c *Client) ListResourceTemplates(ctx context.Context, cursor string) (ResourceTemplatesListResult, error)` — baseline `mcp/client.go:1546`
- `func (c *Client) GetPrompts(ctx context.Context) iter.Seq2[Prompt, error]` — baseline `mcp/client.go:1568`
- `func (c *Client) ListPrompts(ctx context.Context, cursor string) (PromptsListResult, error)` — baseline `mcp/client.go:1594`
- `func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptsGetResult, error)` — baseline `mcp/client.go:1616`
- `func (c *Client) GetPromptRound(` — baseline `mcp/client.go:1625`
- `func (c *Client) Listen(ctx context.Context, filter SubscriptionFilter) (*Subscription, error)` — baseline `mcp/client.go:1694`
- `func (c *Client) Invalidations() <-chan Invalidation` — baseline `mcp/client.go:2206`
- `func (c *Client) InvalidationGeneration() uint64` — baseline `mcp/client.go:2207`
- `func (c *Client) DiscoveryGeneration(kind InvalidationKind) uint64` — baseline `mcp/client.go:2208`
- `func (c *Client) ServerInfo() DiscoverResult` — baseline `mcp/client.go:2221`
- `func (c *Client) Close() error` — baseline `mcp/client.go:2235`

## content.go

- `func FormatContentBlocks(blocks []ContentBlock) ([]byte, error)` — baseline `mcp/content.go:106`
- `func FormatResourceContents(contents []ResourceContents) ([]byte, error)` — baseline `mcp/content.go:111`

## errors.go

- `type ProtocolVersionError struct` — baseline `mcp/errors.go:33`
- `func (e *ProtocolVersionError) Error() string` — baseline `mcp/errors.go:38`
- `type CapabilityError struct` — baseline `mcp/errors.go:46`
- `func (e *CapabilityError) Error() string` — baseline `mcp/errors.go:48`
- `type UnsupportedFeatureError struct` — baseline `mcp/errors.go:52`
- `func (e *UnsupportedFeatureError) Error() string` — baseline `mcp/errors.go:54`
- `type StaleDiscoveryError struct` — baseline `mcp/errors.go:58`
- `func (e *StaleDiscoveryError) Error() string` — baseline `mcp/errors.go:64`
- `type InvalidPayloadError struct` — baseline `mcp/errors.go:73`
- `func (e *InvalidPayloadError) Error() string` — baseline `mcp/errors.go:78`
- `func (e *InvalidPayloadError) Unwrap() error` — baseline `mcp/errors.go:86`
- `func (e *InvalidPayloadError) Is(target error) bool` — baseline `mcp/errors.go:88`
- `type RPCError struct` — baseline `mcp/errors.go:90`
- `type HeaderMismatchError struct` — baseline `mcp/errors.go:97`
- `func (e *HeaderMismatchError) Error() string` — baseline `mcp/errors.go:99`
- `type MissingRequiredClientCapabilityError struct` — baseline `mcp/errors.go:104`
- `func (e *MissingRequiredClientCapabilityError) Error() string` — baseline `mcp/errors.go:109`
- `type UnsupportedProtocolVersionError struct` — baseline `mcp/errors.go:114`
- `func (e *UnsupportedProtocolVersionError) Error() string` — baseline `mcp/errors.go:120`
- `func TypedRPCError(rpc *RPCError) error` — baseline `mcp/errors.go:130`
- `func (e *RPCError) Error() string` — baseline `mcp/errors.go:192`
- `type HTTPError struct` — baseline `mcp/errors.go:196`
- `type TransportCrashError struct` — baseline `mcp/errors.go:203`
- `func (e *TransportCrashError) Error() string` — baseline `mcp/errors.go:208`
- `func (e *TransportCrashError) Unwrap() error` — baseline `mcp/errors.go:220`
- `func (e HTTPError) Error() string` — baseline `mcp/errors.go:222`
- `type RemoteToolError struct` — baseline `mcp/errors.go:226`
- `func (e *RemoteToolError) Error() string` — baseline `mcp/errors.go:231`

## extensions.go

- `type ExtensionCodec interface` — baseline `mcp/extensions.go:14`
- `type ExtensionRegistry struct` — baseline `mcp/extensions.go:23`
- `func (r *ExtensionRegistry) Register(identifier string, codec ExtensionCodec) error` — baseline `mcp/extensions.go:28`
- `func (r *ExtensionRegistry) Decode(identifier string, raw json.RawMessage) (any, error)` — baseline `mcp/extensions.go:47`
- `func (r *ExtensionRegistry) Encode(identifier string, value any) (json.RawMessage, error)` — baseline `mcp/extensions.go:60`

## http_auth.go

- `type HTTPAuthChallenge struct` — baseline `mcp/http_auth.go:20`
- `func (c HTTPAuthChallenge) String() string` — baseline `mcp/http_auth.go:27`
- `func (c HTTPAuthChallenge) LogValue() slog.Value` — baseline `mcp/http_auth.go:29`
- `func (c HTTPAuthChallenge) Format(state fmt.State, _ rune)` — baseline `mcp/http_auth.go:33`
- `func (e HTTPError) Format(state fmt.State, _ rune)` — baseline `mcp/http_auth.go:36`
- `func (e HTTPError) LogValue() slog.Value` — baseline `mcp/http_auth.go:38`

## iterators.go

- `type PaginationLimits struct` — baseline `mcp/iterators.go:17`
- `func IterateCursor[T any](` — baseline `mcp/iterators.go:57`
- `func IterateCursorWithLimits[T any](` — baseline `mcp/iterators.go:69`

## peer.go

- `func (p *pendingRequest) ID() json.RawMessage` — baseline `mcp/peer.go:54`
- `func (p *pendingRequest) DeliveryDone() <-chan struct` — baseline `mcp/peer.go:56`
- `func (p *pendingRequest) WasSent() bool` — baseline `mcp/peer.go:57`
- `func (p *pendingRequest) CancelPending() bool` — baseline `mcp/peer.go:59`
- `func (p *pendingRequest) ClaimCancellationNotification() bool` — baseline `mcp/peer.go:66`
- `func (p *pendingRequest) Await(ctx context.Context) (json.RawMessage, error)` — baseline `mcp/peer.go:79`
- `func (p *pendingRequest) OnComplete(hook func())` — baseline `mcp/peer.go:118`

## policy.go

- `type ToolAnnotations struct` — baseline `mcp/policy.go:21`
- `type ToolExecutionProperties struct` — baseline `mcp/policy.go:33`
- `type ToolPolicyMapper func(context.Context, MCPTool) (ToolExecutionProperties, error)` — baseline `mcp/policy.go:44`
- `func WithToolPolicyMapper(mapper ToolPolicyMapper) ClientOption` — baseline `mcp/policy.go:46`
- `func (t *generationTool) SupportsPreparedExecution() bool` — baseline `mcp/policy.go:105`
- `func (t *generationTool) Execute(` — baseline `mcp/policy.go:107`

## protocol.go

- `type CacheScope string` — baseline `mcp/protocol.go:29`
- `type Meta map[string]json.RawMessage` — baseline `mcp/protocol.go:52`
- `func (m *Meta) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol.go:54`
- `func (m Meta) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol.go:66`
- `type JSONNumber string` — baseline `mcp/protocol.go:89`
- `func (n *JSONNumber) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol.go:91`
- `func (n JSONNumber) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol.go:98`
- `func (n JSONNumber) String() string` — baseline `mcp/protocol.go:104`
- `type ProgressToken struct` — baseline `mcp/protocol.go:107`
- `func NewStringProgressToken(value string) ProgressToken` — baseline `mcp/protocol.go:111`
- `func NewIntegerProgressToken(value int64) ProgressToken` — baseline `mcp/protocol.go:115`
- `func (t ProgressToken) IsZero() bool` — baseline `mcp/protocol.go:118`
- `func (t ProgressToken) String() string` — baseline `mcp/protocol.go:119`
- `func (t ProgressToken) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol.go:126`
- `func (t *ProgressToken) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol.go:132`
- `type Request struct` — baseline `mcp/protocol.go:149`
- `type Response struct` — baseline `mcp/protocol.go:155`
- `type Notification struct` — baseline `mcp/protocol.go:161`
- `type JSONRPCError struct` — baseline `mcp/protocol.go:166`
- `type Icon struct` — baseline `mcp/protocol.go:172`
- `type Implementation struct` — baseline `mcp/protocol.go:179`
- `type ClientCapabilities struct` — baseline `mcp/protocol.go:191`
- `type RootsCapability struct` — baseline `mcp/protocol.go:201`
- `type SamplingCapability struct` — baseline `mcp/protocol.go:206`
- `type ElicitationCapability struct` — baseline `mcp/protocol.go:213`
- `type ServerCapabilities struct` — baseline `mcp/protocol.go:218`
- `type ToolsCapability struct` — baseline `mcp/protocol.go:225`
- `type ResourcesCapability struct` — baseline `mcp/protocol.go:229`
- `type PromptsCapability struct` — baseline `mcp/protocol.go:234`
- `type RequestMeta struct` — baseline `mcp/protocol.go:239`
- `type ResultMeta struct` — baseline `mcp/protocol.go:247`
- `type RequestParams struct` — baseline `mcp/protocol.go:251`
- `type NotificationParams struct` — baseline `mcp/protocol.go:254`
- `type DiscoverParams = RequestParams` — baseline `mcp/protocol.go:259`
- `type DiscoverResult struct` — baseline `mcp/protocol.go:261`
- `type CacheInfo struct` — baseline `mcp/protocol.go:272`
- `type CompleteResult struct` — baseline `mcp/protocol.go:276`
- `type EmptyResult = CompleteResult` — baseline `mcp/protocol.go:281`
- `type InputRequiredResult struct` — baseline `mcp/protocol.go:282`
- `type CursorParams struct` — baseline `mcp/protocol.go:290`
- `type ToolsListParams = CursorParams` — baseline `mcp/protocol.go:294`
- `type ToolsListResult struct` — baseline `mcp/protocol.go:296`
- `type MCPTool struct` — baseline `mcp/protocol.go:305`
- `type ToolsCallParams struct` — baseline `mcp/protocol.go:316`
- `type CallToolResult struct` — baseline `mcp/protocol.go:323`
- `type Annotations struct` — baseline `mcp/protocol.go:332`
- `type ResourceContents struct` — baseline `mcp/protocol.go:338`
- `type ContentBlock struct` — baseline `mcp/protocol.go:349`
- `type ResourcesListParams = CursorParams` — baseline `mcp/protocol.go:366`
- `type ResourcesListResult struct` — baseline `mcp/protocol.go:368`
- `type Resource struct` — baseline `mcp/protocol.go:377`
- `type ResourceTemplatesListParams = CursorParams` — baseline `mcp/protocol.go:389`
- `type ResourceTemplatesListResult struct` — baseline `mcp/protocol.go:391`
- `type ResourceTemplate struct` — baseline `mcp/protocol.go:400`
- `type ResourcesReadParams struct` — baseline `mcp/protocol.go:411`
- `type ResourcesReadResult struct` — baseline `mcp/protocol.go:418`
- `type PromptsListParams = CursorParams` — baseline `mcp/protocol.go:427`
- `type PromptsListResult struct` — baseline `mcp/protocol.go:429`
- `type Prompt struct` — baseline `mcp/protocol.go:438`
- `type PromptArgument struct` — baseline `mcp/protocol.go:447`
- `type PromptsGetParams struct` — baseline `mcp/protocol.go:454`
- `type PromptsGetResult struct` — baseline `mcp/protocol.go:461`
- `type PromptMessage struct` — baseline `mcp/protocol.go:468`
- `type SubscriptionFilter struct` — baseline `mcp/protocol.go:474`
- `type SubscriptionsListenParams struct` — baseline `mcp/protocol.go:481`
- `type SubscriptionsListenResult struct` — baseline `mcp/protocol.go:486`
- `type SubscriptionResultMeta struct` — baseline `mcp/protocol.go:491`
- `type SubscriptionsAcknowledgedParams struct` — baseline `mcp/protocol.go:496`
- `type ResourceUpdatedParams struct` — baseline `mcp/protocol.go:501`
- `type LogMessageParams struct` — baseline `mcp/protocol.go:506`
- `type ProgressParams struct` — baseline `mcp/protocol.go:513`
- `type CancelledParams struct` — baseline `mcp/protocol.go:521`

## protocol_validate.go

- `func (m *RequestMeta) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:260`
- `func (m RequestMeta) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:292`
- `func (m *ResultMeta) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:338`
- `func (m ResultMeta) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:356`
- `func (m *SubscriptionResultMeta) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:360`
- `func (m SubscriptionResultMeta) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:385`
- `func (c *ClientCapabilities) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:414`
- `func (c ClientCapabilities) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:436`
- `func (c *RootsCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:457`
- `func (c RootsCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:466`
- `func (c *SamplingCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:471`
- `func (c SamplingCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:488`
- `func (c *ElicitationCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:493`
- `func (c ElicitationCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:510`
- `func (c *ServerCapabilities) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:514`
- `func (c ServerCapabilities) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:552`
- `func (p *RequestParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:572`
- `func (p RequestParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:588`
- `func (p *CursorParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:592`
- `func (p CursorParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:611`
- `func (p *ToolsCallParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:615`
- `func (p ToolsCallParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:647`
- `func (p *ResourcesReadParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:651`
- `func (p ResourcesReadParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:681`
- `func (p *PromptsGetParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:685`
- `func (p PromptsGetParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:715`
- `func (p *SubscriptionsListenParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:719`
- `func (p SubscriptionsListenParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:736`
- `func (f *SubscriptionFilter) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:741`
- `func (f SubscriptionFilter) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:782`
- `func (r *SubscriptionsListenResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:794`
- `func (r SubscriptionsListenResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:814`
- `func (p *SubscriptionsAcknowledgedParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:819`
- `func (p SubscriptionsAcknowledgedParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:836`
- `func (i *Implementation) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:841`
- `func (i Implementation) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:871`
- `func (i *Icon) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:877`
- `func (i Icon) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:905`
- `func (c *ToolsCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:909`
- `func (c ToolsCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:923`
- `func (c *ResourcesCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:927`
- `func (c ResourcesCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:944`
- `func (c *PromptsCapability) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:948`
- `func (c PromptsCapability) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:962`
- `func (r *DiscoverResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:967`
- `func (r DiscoverResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1018`
- `func (r *ToolsListResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1099`
- `func (r ToolsListResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1120`
- `func (r *ResourcesListResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1133`
- `func (r ResourcesListResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1154`
- `func (r *ResourceTemplatesListResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1167`
- `func (r ResourceTemplatesListResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1188`
- `func (r *ResourcesReadResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1201`
- `func (r ResourcesReadResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1221`
- `func (r *PromptsListResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1233`
- `func (r PromptsListResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1254`
- `func (r *PromptsGetResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1267`
- `func (r PromptsGetResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1278`
- `func (r *CallToolResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1296`
- `func (r CallToolResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1333`
- `func (r *InputRequiredResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1345`
- `func (r InputRequiredResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1383`
- `func (r *CompleteResult) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1394`
- `func (r CompleteResult) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1403`
- `func (t *MCPTool) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1470`
- `func (t MCPTool) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1530`
- `func (r *ResourceContents) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1538`
- `func (r ResourceContents) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1579`
- `func (r *Resource) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1586`
- `func (r Resource) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1636`
- `func (r *ResourceTemplate) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1652`
- `func (r ResourceTemplate) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1812`
- `func (a *Annotations) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1828`
- `func (a Annotations) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1862`
- `func (a *ToolAnnotations) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1867`
- `func (a ToolAnnotations) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1889`
- `func (b *ContentBlock) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:1896`
- `func (b ContentBlock) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:1970`
- `func (p *PromptArgument) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2084`
- `func (p PromptArgument) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2106`
- `func (p *Prompt) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2110`
- `func (p Prompt) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2137`
- `func (p *PromptMessage) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2143`
- `func (p PromptMessage) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2163`
- `func (p *NotificationParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2168`
- `func (p NotificationParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2182`
- `func (p *ResourceUpdatedParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2187`
- `func (p ResourceUpdatedParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2207`
- `func (p *LogMessageParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2212`
- `func (p LogMessageParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2242`
- `func (p *ProgressParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2247`
- `func (p ProgressParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2270`
- `func (p *CancelledParams) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2277`
- `func (p CancelledParams) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2300`
- `func (r *Request) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2319`
- `func (r Request) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2341`
- `func (n *Notification) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2345`
- `func (n Notification) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2364`
- `func (e *JSONRPCError) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2368`
- `func (e JSONRPCError) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2387`
- `func (r *Response) UnmarshalJSON(data []byte) error` — baseline `mcp/protocol_validate.go:2391`
- `func (r Response) MarshalJSON() ([]byte, error)` — baseline `mcp/protocol_validate.go:2418`

## snapshot.go

- `type Snapshot interface` — baseline `mcp/snapshot.go:24`
- `type SnapshotDigest [sha256.Size]byte` — baseline `mcp/snapshot.go:31`
- `func (d SnapshotDigest) String() string` — baseline `mcp/snapshot.go:33`
- `func ComputeSnapshotDigest(snapshot Snapshot) (SnapshotDigest, error)` — baseline `mcp/snapshot.go:45`

## subscription_retirement.go

- `type SubscriptionLimits struct` — baseline `mcp/subscription_retirement.go:13`
- `func WithSubscriptionLimits(limits SubscriptionLimits) ClientOption` — baseline `mcp/subscription_retirement.go:38`

## tool_discovery.go

- `type ToolDiscovery struct` — baseline `mcp/tool_discovery.go:12`

## transport.go

- `type PendingRequest interface` — baseline `mcp/transport.go:36`
- `type PreparedRequest interface` — baseline `mcp/transport.go:43`
- `func (p *preparedRequest) Deliver() error` — baseline `mcp/transport.go:66`
- `func (p *preparedRequest) Abort(cause error) error` — baseline `mcp/transport.go:85`
- `type CompletionPendingRequest interface` — baseline `mcp/transport.go:99`
- `type DeliveryPendingRequest interface` — baseline `mcp/transport.go:106`
- `type CancellablePendingRequest interface` — baseline `mcp/transport.go:113`
- `type CancellationNotificationPending interface` — baseline `mcp/transport.go:120`
- `type NotificationHandler func(params json.RawMessage)` — baseline `mcp/transport.go:125`
- `type RequestScopedNotificationHandler func(requestID json.RawMessage, params json.RawMessage)` — baseline `mcp/transport.go:129`
- `type RequestScopedNotificationTransport interface` — baseline `mcp/transport.go:133`
- `type Transport interface` — baseline `mcp/transport.go:139`
- `type HTTPToolHeaderBinding struct` — baseline `mcp/transport.go:148`
- `type ToolHeaderTransport interface` — baseline `mcp/transport.go:158`
- `type FrameByteCapTransport interface` — baseline `mcp/transport.go:162`

## transport_http.go

- `type HTTPRequestDecorator func(*http.Request) error` — baseline `mcp/transport_http.go:41`
- `type StreamableHTTPOption func(*StreamableHTTPTransport)` — baseline `mcp/transport_http.go:42`
- `func WithStreamableHTTPLogger(logger *slog.Logger) StreamableHTTPOption` — baseline `mcp/transport_http.go:44`
- `func WithStreamableHTTPAllowPrivateIPs(allow bool) StreamableHTTPOption` — baseline `mcp/transport_http.go:52`
- `func WithStreamableHTTPSettings(settings httptool.ClientSettings) StreamableHTTPOption` — baseline `mcp/transport_http.go:58`
- `func WithStreamableHTTPRequestDecorator(decorator HTTPRequestDecorator) StreamableHTTPOption` — baseline `mcp/transport_http.go:62`
- `type StreamableHTTPTransport struct` — baseline `mcp/transport_http.go:66`
- `func NewStreamableHTTPTransport(endpoint string, opts ...StreamableHTTPOption) *StreamableHTTPTransport` — baseline `mcp/transport_http.go:96`
- `func (t *StreamableHTTPTransport) Start(ctx context.Context) error` — baseline `mcp/transport_http.go:129`
- `func (t *StreamableHTTPTransport) PrepareRequest(` — baseline `mcp/transport_http.go:163`
- `func (t *StreamableHTTPTransport) Notify(_ context.Context, method string, params any) error` — baseline `mcp/transport_http.go:213`
- `func (t *StreamableHTTPTransport) OnNotification(method string, handler NotificationHandler)` — baseline `mcp/transport_http.go:223`
- `func (t *StreamableHTTPTransport) OnRequestNotification(` — baseline `mcp/transport_http.go:237`
- `func (t *StreamableHTTPTransport) ReplaceToolHeaderBindings(` — baseline `mcp/transport_http.go:279`
- `func (t *StreamableHTTPTransport) MaxFrameBytes() int` — baseline `mcp/transport_http.go:1364`
- `func (t *StreamableHTTPTransport) Close() error` — baseline `mcp/transport_http.go:1375`

## transport_limits.go

- `type TransportLimits struct` — baseline `mcp/transport_limits.go:19`
- `func WithStdioLimits(limits TransportLimits) StdioTransportOption` — baseline `mcp/transport_limits.go:45`
- `func WithStreamableHTTPLimits(limits TransportLimits) StreamableHTTPOption` — baseline `mcp/transport_limits.go:49`
- `type TransportLimitError struct` — baseline `mcp/transport_limits.go:62`
- `func (e *TransportLimitError) Error() string` — baseline `mcp/transport_limits.go:67`
- `func (e *TransportLimitError) Unwrap() error` — baseline `mcp/transport_limits.go:71`

## transport_stdio.go

- `type StdioTransportOption func(*StdioTransport)` — baseline `mcp/transport_stdio.go:29`
- `func WithStdioLogger(logger *slog.Logger) StdioTransportOption` — baseline `mcp/transport_stdio.go:55`
- `type StdioTransport struct` — baseline `mcp/transport_stdio.go:63`
- `func NewStdioTransport(` — baseline `mcp/transport_stdio.go:99`
- `func (t *StdioTransport) Start(ctx context.Context) error` — baseline `mcp/transport_stdio.go:137`
- `func (t *StdioTransport) PrepareRequest(` — baseline `mcp/transport_stdio.go:413`
- `func (t *StdioTransport) Notify(ctx context.Context, method string, params any) error` — baseline `mcp/transport_stdio.go:459`
- `func (t *StdioTransport) OnNotification(method string, handler NotificationHandler)` — baseline `mcp/transport_stdio.go:473`
- `func (t *StdioTransport) MaxFrameBytes() int` — baseline `mcp/transport_stdio.go:603`
- `func (t *StdioTransport) Close() error` — baseline `mcp/transport_stdio.go:614`

## Baseline test files

Files below are baseline history; deleted protocol tests are replaced by the new contract suites. Replacement/retirement requires matching evidence from the new contract; old protocol-only tests may be intentionally delegated, with documented disposition.

- `adversarial_client_migration_test.go` (baseline): TestClient_NonToolCancellationUsesActiveRequestID, TestClient_ToolsInvalidationAbortsInFlightSnapshot, TestClient_TerminalYieldAbortDoesNotSendObsoleteCancellation
- `adversarial_contract_test.go` (baseline): TestSSERetryValuesCannotEnableReplay, TestStreamableHTTP_ForbiddenIncomingRequestCannotSelfDeadlock, TestStreamableHTTP_CancellationClosesOriginalWireRequestWithoutNotification, TestStreamableHTTP_DiscoveryDisconnectCannotResumeOrInitialize, TestStreamableHTTP_SessionAdvertisementsAreInert, TestStreamableHTTP_DecoratorTargetGuardAndInertEventIDs, TestStreamableHTTP_ParallelDisconnectedPOSTsNeverReplay, TestStreamableHTTP_DisconnectFailsBeforeResume405, TestStreamableHTTP_CancelledPendingStopsOriginPOSTWithoutResumeGET, TestStreamableHTTP_ConnectNeverActivatesResumeCursor, TestStreamableHTTP_ForgedSessionHeadersCannotBeDecorated, TestStreamableHTTP_OversizedPOSTSSEFailsClosedWithoutRetry, TestStreamableHTTP_TerminalFailureCancelsConcurrentPOSTLifecycle, TestStreamableHTTP_NotificationsForbiddenRegardlessOfResponseShape
- `adversarial_dto_migration_test.go` (baseline): TestProtocol_StrictRequiredFieldsAndTaggedUnions
- `adversarial_http_lifecycle_migration_test.go` (baseline): TestStreamableHTTP_CloseCompletesHeldRequestWithTransportClosed, TestStreamableHTTP_PreCancelledRequestNeverAllocatesOrWritesID, TestStreamableHTTP_DecoratorCannotOverrideRequestHost, TestStreamableHTTP_DecoratorCannotSetBodyContract, TestStreamableHTTP_RequestIsDetachedFromDecoratorPointer, TestStreamableHTTP_RejectsPrivateEndpointAndOversizedBody, TestStreamableHTTP_RequestRejects202AndCloseCancelsActivePOST, TestStreamableHTTP_JSONPOSTRequiresCorrelatedTerminalResponse, TestStreamableHTTP_RejectsInvalidUTF8JSONAndSSE, TestStreamableHTTP_TerminalSSEEventCancelsHeldOpenPOST, TestStreamableHTTP_RequiresExactMediaType, TestStreamableHTTP_RejectsMethodChangingRedirect, TestStreamableHTTP_SafeClientRejectsRedirectToLoopback, TestTransport_CloseBeforeStartIsTerminal, TestStdio_ProcessExitReturnsTypedCrash, TestStructuredContent_PreservesLargeInteger, TestResourceResult_PreservesSingleContentMIMEAndBinary
- `adversarial_peer_migration_test.go` (baseline): TestRPCPeer_BeginRequestCloseRaceNeverLeaks, TestRPCPeer_IncomingRequestsAreForbiddenUnderConcurrentLoad, TestRPCPeer_RejectsOversizedRequestIDBeforeNumericCanonicalization, TestRPCPeer_ResponseCancellationRaceConsumesOneTerminalResponse, TestRPCPeer_ExternalCancellationUnblocksBackgroundAwait, TestRPCPeer_CancelledResponseBookkeepingRemainsBounded, TestRPCPeer_CancellationStormNeverMasksDuplicateCompletedResponse, TestRPCPeer_OutOfOrderCancellationsRemainExactlyCorrelated, TestRPCPeer_CancellationRangeInsertionAndBridgingStaySorted, TestRPCPeer_CancellationRangeFragmentationFailsClosedAtBound, TestRPCPeer_LateCancelledResponseSplitsRangeAndDuplicateFailsClosed, TestRPCPeer_DuplicateResponseFailsClosedAndNotifyAfterCloseStops
- `adversarial_sse_framing_migration_test.go` (baseline): TestStreamableHTTP_EmptySSEDataDoesNotCompleteRequest, TestStreamableHTTP_SSEAcceptsStandardLineEndingsAndLeadingBOM, TestStreamableHTTP_SSEDiscardsIncompleteEventAtEOF, TestStreamableHTTP_SSELineUsesConfiguredStreamLimit
- `client_contract_test.go` (baseline): TestConnect_ExactVersionWithoutRootsAuthority, TestConnect_DiscoveryPublishesReadyWithoutLegacyNotification, TestLegacyRootsRejectNonCanonicalTypedFileURIs, TestLegacyRootsDoNotCanonicalizeTypedFilePath, TestLegacyRootsDoNotConvertWindowsDrivePaths, TestLegacyRootsRejectUNCPaths, TestLegacyRootsDoNotCanonicalizeTypedDriveURIs, TestLegacyRootsRejectDriveRelativeTypedURIs, TestLegacyRootsRejectTypedUNCAliases, TestClient_LegacyPingRejectedBeforeAndAfterDiscovery, TestClient_LegacyPingAndRootsRejectMalformedRequestMeta, TestConnect_VersionMismatchFailsWithoutFallback, TestClient_InputRootsSnapshotOwnsMetadataWithoutRootsService, TestClient_CancellationDeliveryWaitIsBounded, TestClient_DoesNotCancelTerminalPendingRequest, TestValidatePrompt_AllowsHumanReadableName, TestClient_MissingCapabilityRejectedWithoutRPC, TestClient_ToolStructuredResultMapsOutputSchemaAndTypedEnvelope, TestClient_ToolResultViolatingOutputSchemaFailsClosed, TestClient_OutputSchemaPreservesExactNumericConstraints, TestClient_RemoteToolErrorHasDistinctEnvelope, TestClient_CancellationUsesActiveRequestIDExactlyOnce, TestClient_ProgressIsFractionalAndMonotonic
- `client_features_test.go` (baseline): TestClient_InvalidListChangedMetaDoesNotAdvanceGeneration, TestClient_SubscribeResourceRequiresObjectResultBeforeMutation, TestClient_SubscribeResourceAcceptsExtendedEmptyResult, TestClient_CloseIsConcurrentIdempotentAndClosesEventChannels, TestClient_ToolDescriptorFailuresAreFailClosed, TestClient_ToolsListChangedMakesExistingProxyStale, TestClient_ContradictingInvalidationIsIgnored, TestClient_ResourceUpdateRequiresNegotiatedSubscription, TestClient_ResourceSubscriptionAcceptsSubResourceUpdate, TestResourceUpdateMatchesSubscriptionURIBoundaries, TestClient_LoggingNotificationAllowsNullData, TestClient_LoggingNotificationRequiresCapabilityAndPreservesData, TestClient_PromptTaggedContentAndMetadata
- `client_semantics_test.go` (baseline): TestPrepareParamsRejectsUnsupportedShapeAndAcceptsLoggingOptIn, TestSharedTransportRejectsRequestScopedLoggingBeforePrepare, TestContextualHTTPLoggingDoesNotRequireProgressToken, TestAwaitCancellationNotifiesExactlyOnceAfterDelivery, TestDuplicateActiveRequestLogIDIsRejected, TestProgressTokenKeyPreservesJSONDiscriminator, TestListToolsFiltersAndRegistersHeaderBindings, TestHTTPDirectCallRequiresCurrentAuthoritativeToolDescriptor, TestDuplicateToolNamesNeverBecomeRoutingAuthority, TestPaginatedToolAuthorityPublishesOnlyAfterCompleteTransaction, TestPublicCallToolValidatesAuthoritativeOutputSchema, TestWithClientInfoDeepClonesExtensionMetadata, TestConnectResnapshotsClientInfoAfterCustomOption, TestListenSnapshotsFilterSlicesAndExtensionMetadata, TestSubscriptionResourceViolationDoesNotDeadlock, TestEffectiveSubscriptionFilterMustBeStrictSubset, TestListenRegistersStateBeforeRequestDelivery, TestListenClosedClientAbortsWithoutWireDelivery, TestDirectAwaitMapsReservedRPCError, TestResourceProxyRejectsStaleGenerationBeforeRPC, TestDirectToolCallRejectsInvalidationDuringRPC, TestDirectListAPIsRejectInvalidationDuringRPC, TestResourceSubscriptionAllowsOnlyExactOrPathDescendantURI
- `client_test.go` (baseline): TestNotifyCancelledRequestUsesBoundedContext, TestConnect_EagerHandshakeSuccess, TestConnect_StartFailureClosesTransport, TestConnect_DiscoveryCallFailureClosesTransport, TestConnect_DiscoveryParseFailureClosesTransport, TestConnect_LegacyInitializedNotificationIsAbsent, TestToolResult_ErrorChunkPreservesTypedRemoteEnvelope, TestHandleToolCallResult_ReadLimitExceeded_MapsValidation, TestGetResourceTool_ReadLimitExceeded, TestGetPrompt_ReadLimitExceeded, TestGetResourceTool_ReadLimit_CustomStreamCap, TestHandleToolCallResult_ReadLimit_CustomStreamCap, TestGetTools_RemoteAnnotationsRemainUntrusted, TestConnect_Discovery_ReadLimitMapsValidation, TestGetTools_ReadLimitExceeded, TestGetPrompts_ReadLimitExceeded, TestConnect_Discovery_ReadLimit_CustomStreamCap, TestGetTools_ReadLimit_CustomStreamCap, TestGetPrompts_ReadLimit_CustomStreamCap, TestMapCallReadLimit_CancelOverLimit, TestClient_ListBoundariesMapReadLimitsAndPreserveInterrupts, TestMapCallReadLimit_DeadlineOverLimit, TestMapCallReadLimit_TimeoutOverLimit, TestHandleToolCallResult_CancelOverReadLimit, TestMapCallReadLimitFor_InterruptInChainOverReadLimit, TestHandleToolCallResult_InterruptInChainOverReadLimit
- `content_test.go` (baseline): TestFormatContentBlocks_BinaryDataIsRetainedInProjection, TestFormatContentBlocks_UnknownTypeFailsClosed, TestFormatResourceContents_RejectsAmbiguousUnion
- `custom_pending_abort_migration_test.go` (baseline): TestCustomPendingConsumerAbortCleansInvocationWithoutClosingTransport
- `errors_test.go` (baseline): TestProtocolVersionErrorBoundsServerDiagnostic, TestJSONRPCErrorRejectsFractionalCodeOnMarshal
- `fixture_contract_test.go` (baseline): TestTask34OfficialResultFixturesDecodeStrictly, TestTask34AdversarialCacheAndDuplicateFixturesFailClosed, TestTask34OfficialRequestAndHTTPFixtures, TestTask34OfficialSubscriptionSequence, TestTask34AdversarialDiscoveryAndResultUnionFixtures, TestTask34AdversarialHeaderAndSchemaFixtures, TestTask34AdversarialSubscriptionPeerAndLegacyMatrices, TestTask34FixtureInventoryIsComplete
- `http_auth_test.go` (baseline): TestHTTPAuthenticationFailurePreservesSafeHintsWithoutRetry, TestAuthenticationChallengeBoundsAndUnsafeParameters, TestAuthenticationFailureDoesNotDrainUnfinishedBody
- `iterators_test.go` (baseline): TestIterateCursorWithLimits_RejectsUnboundedUniquePages, TestIterateCursorWithLimits_RejectsCumulativeCursorBytes
- `pagination_cache_task34_test.go` (baseline): TestTask34PaginationPreservesChangingPerPageCacheSnapshots
- `peer_generated_task34_test.go` (baseline): TestTask34RandomizedIntegralRPCIDsCanonicalizeWithoutStringCollision, TestTask34CancelledIDFragmentationRemainsBoundedAndFailsClosed
- `peer_test.go` (baseline): TestRPCPeer_RequestAndNotificationWithSameMethodAreDistinct, TestRPCPeer_UnknownIncomingMethodFailsClosed, TestRPCPeer_StringAndNumericIDsDoNotCollide, TestRPCPeer_EmptyMethodFailsClosed, TestRPCPeer_HybridRequestResponseEnvelopeIsRejected, TestRPCPeer_FractionalIncomingRequestIDIsRejectedWithoutDispatch, TestRPCPeer_MalformedIncomingRequestsFailClosed, TestRPCPeer_JSONRPCErrorCodePreservesExactIntegers, TestRPCPeer_JSONRPCErrorCodeRejectsFraction, TestRPCPeer_NonObjectAndInvalidJSONFailClosed, TestRPCPeer_MalformedNotificationShapedMessagesNeverReceiveResponse, TestRPCPeer_CloseAfterForbiddenIncomingRequest, TestRPCPeer_InvalidErrorObjectCompletesCorrelatedRequest, TestRPCPeer_RejectsNonObjectParams, TestRPCPeer_CloseUnblocksEveryPendingRequest
- `policy_cache_test.go` (baseline): TestRemoteHintsRequireHostTrustForCacheAndCurrentPolicy, TestToolPolicyMapperRefusalPreventsProxyDelivery
- `policy_test.go` (baseline): TestGetResourceTool_ReadOnlyManifest
- `pool_test.go` (baseline): TestPublicMCPPoolReuseAndCleanup, TestMCPHTTPSettingsFailBeforeDispatch
- `preflight_contract_test.go` (baseline): TestProductionProtocolHasNoLegacyFallback
- `process_exists_other_test.go` (baseline): 
- `process_exists_unix_test.go` (baseline): 
- `process_exists_windows_test.go` (baseline): 
- `progress_terminal_migration_test.go` (baseline): TestMigratedProgressRetiresBeforeAwaitAndDrainsAcceptedEvents, TestMigratedMCPConsumerAbortPreservesCause, TestMigratedEmptyResourceChunksRespectCoreContract, TestMigratedProgressMissingCompletionAbortsBeforeDelivery
- `protocol_boundary_task34_test.go` (baseline): TestProtocolBoundaryRejectsRecursiveDuplicateKeys, TestCacheInfoPreservesUnboundedIntegerAndRejectsInvalidForms, TestMRTRRequestStateIsByteExactOpaqueString, TestMalformedServerInfoIsDisplayMetadataAndTreatedAbsent, TestContentBlockTaggedUnionIsSymmetric, TestReservedRPCErrorDataViolationsAreClassified, TestCurrentInertCapabilitiesAndUnrelatedExtrasRemainLossless, TestExtensionRegistryRequiresExplicitOwnership, TestResultUnionRejectsForeignVariantFieldsAcrossMethods, TestTask34AdversarialResultUnionFixtureFailClosed, TestOutboundExtraAndMetaRejectRecursiveDuplicateJSON, TestSubscriptionMetaIgnoresMalformedServerInfo, TestMCPToolCompilesSchemasAtWireBoundary, TestCapabilityExtensionsPreserveCurrentMCPNamespace, TestStrictDTOFamiliesRejectNullAndPreserveAdditiveFields, TestMetaIdentifiersRejectEmptyAndPrefixOnlyKeys, TestCallerRequestMetaRejectsAllReservedSecondLabelNamespaces, TestContentBlockVariantsRejectNullWrongTypeAndForeignOptionals, TestCurrentClientCapabilityShapesAreTypedStrictAndInert, TestMissingRequiredCapabilityErrorDecodesCurrentTypedShapes, TestResourceSizesRequireLosslessJSONIntegersSymmetrically, TestKnownOptionalWireFieldsRejectPresentNullAndInvalidEnums, TestResourceTemplateValidatesRFC6570ExpressionsSymmetrically
- `protocol_contract_test.go` (baseline): TestResourceContents_HasNoNonStandardAnnotationsSurface, TestResourceContents_UnknownAnnotationFieldsRemainInert, TestProgressToken_StringAndNumberWireContract, TestEmptyResult_RequiresObjectAndPreservesExtensions, TestMeta_KeyFormatIsValidatedOnBothDirections, TestRequestMeta_ExtensionKeyFormatIsValidated, TestProgressToken_RejectsNullAndCompositeValues, TestCallToolResult_AllContentVariantsRemainLossless, TestContentBlock_EmptyRequiredPayloadRoundTrips, TestResourceLinkSize_PreservesIntegersAndRejectsFraction, TestStrictDTOsRejectPresentNullAndFractionalInteger, TestRequestDTOsRoundTripTypedProgressMetaAndExtra, TestRequestMetaRejectsNullAndResetsOnReuse, TestRequestMetaRejectsReservedExtraCollision, TestResourceContents_TextAndBlobAreDistinct, TestServerCapabilities_PreservesUnknownAdditiveCapabilities, TestResultDTOsPreserveUnknownTopLevelExtensions, TestResultDTORejectsExtensionCollisionWithReservedField, TestServerCapabilities_KnownCapabilitiesMustBeObjects, TestPrimitiveMetadata_RoundTripsOnCurrentWireDTOs
- `protocol_cycle7_task34_test.go` (baseline): TestPinnedInputRequestUnionRejectsMalformedVariants, TestPinnedInputResponseUnionRunsBeforeAllOutboundMethods, TestInvalidInputResponsesFailBeforeCapabilityChecks, TestCanonicalBase64RejectsAlternateSpellingsAndProjectionBypass
- `redirect_effect_test.go` (baseline): TestHTTPRPCRejectsSameOriginBodyReplay
- `schema_contract_cycle7_task34_test.go` (baseline): TestTask34PinnedSchemaRequiresIntegerResourceSizes, TestTask34PinnedSchemaMRTRInputUnionsAreExact, TestTask34CanonicalBase64RejectsNonCanonicalSpellings, TestTask34ReservedHTTPErrorStatusCoherence, TestTask34StdioRejectsOversizedOutgoingFrameBeforeWrite
- `schema_provenance_task34_test.go` (baseline): TestTask34OfficialSchemaArtifactHasImmutableProvenance
- `snapshot_test.go` (baseline): TestComputeSnapshotDigestSupportsEveryCacheableSnapshot, TestComputeSnapshotDigestCanonicalizesMapsNumbersAndPointers, TestComputeSnapshotDigestIncludesCacheMetadataAndWireOrder, TestComputeSnapshotDigestRejectsInvalidAndNilSnapshots, TestComputeSnapshotDigestRejectsOversizedEncodingBeforeCanonicalization
- `sse_framing_task34_test.go` (baseline): TestTask34SSEAcceptsBOMLineEndingsAndFragmentedFrames, TestTask34SSERejectsTruncatedUTF8AcrossFragments
- `sse_line_limit_migration_test.go` (baseline): TestStreamableHTTP_SSELineBudgetOwnsScannerFailure
- `stdio_process_task34_test.go` (baseline): TestTask34StdioCrashUnblocksWaiterAndReapsProcess, TestTask34StdioConcurrentCloseKillsDescendantProcessTree, TestTask34StdioHelperProcess
- `subscription_retirement_test.go` (baseline): TestCanceledSubscriptionLateMessagesLeaveSiblingLive, TestSubscriptionRetirementSaturationAndExpiry, TestRetiredSubscriptionMalformedResourceStillFailsStrictly, TestCancelInProgressStillValidatesMalformedResource
- `subscription_stress_task34_test.go` (baseline): TestTask34SubscriptionOverflowAdvancesAuthorityDuringConcurrentClose
- `subscription_uri_adversarial_migration_test.go` (baseline): TestAuditedSubscriptionURIScopeBoundaries
- `task34_contract_test.go` (baseline): TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata, TestConnectRejectsOldOnlyServerWithoutFallback, TestRequestMetaRejectsReservedInjectionAndDuplicateKeys, TestProtocolRequiresCurrentResultAndCacheFields, TestResultTypeAndStructuredContentAreStrictAndLossless, TestUndeclaredMRTRCapabilityDoesNotRetry, TestTypedReservedRPCErrors, TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders, TestHTTPToolHeaderCompilerRejectsDynamicAndCaseCollidingAnnotations, TestResourceAndPromptListAPIsPreserveCacheMetadata, TestSubscriptionRequiresAcknowledgementBeforeEvents
- `test_support_migration_test.go` (baseline): TestMigratedFakePreparationIsInertAndDetached, TestMigratedFakeAbortCompletesOnceWithoutTraffic, TestMigratedFakeCancellationOwnsOneNotification, TestMigratedFakeRacingCompletionRunsHookOnce, TestMigratedFakeCloseUnblocksPreparedAndDelivered
- `test_support_test.go` (baseline): 
- `tool_discovery_limits_test.go` (baseline): TestFullToolDiscoveryBudgetsPreserveExistingAuthority, TestToolDiscoveryItemBudgetPrecedesSchemaCompilation, TestToolDiscoveryInvalidInputSchemaCannotPublishAuthority, TestClientNegativeAndNilOptionsFailBeforeTransportStart, TestDiscoveryCancellationAfterCommitStarts, TestDiscoveryCanceledBeforePublicationRetainsAuthority
- `transport_http_migration_test.go` (baseline): TestMigratedSSEUsesNoGETOrEndpointDiscovery, TestMigratedSSEPrivateEndpointFailsBeforeTraffic, TestMigratedSSEByteLimitRemainsCumulative, TestMigratedSSECancelUnblocksRequestStream, TestMigratedSSEInterruptPrecedenceAtClientBoundary
- `transport_http_revision_test.go` (baseline): TestPreparedRequestRegistersSubscriptionBeforeImmediateACK, TestPreparedRequestDeliveryBarrierIsExactlyOnce, TestStreamableHTTPPreparedRequestSnapshotsToolRoutingHeaders, TestStreamableHTTPCancellationClosesOnlyOriginatingPOST, TestStdioPostDeliveryCancellationIsEmittedExactlyOnce, TestStdioBlockingWriteCancellationPreservesDeliveredFrameAndTransport, TestStdioCancellationRetractsQueuedRequestWithoutNotification, TestStdioBlockingWriteCancellationRaceCount, TestStdioOutgoingFrameLimitRejectsRequestAndNotificationBeforeEnqueue, TestStdioWriteQueueByteBudgetReleasesAcrossSixtyFourCancellationRaces, TestRequestScopedSSEFailsOnEOFAndByteLimit, TestHTTPRequestScopedLogUsesStreamRequestIDWithoutWireToken, TestTransportBoundaryRejectsEnvelopeBypassForHTTPAndStdio, TestTransportBoundaryRejectsLegacyMethodsAndNotifications, TestPeerFailsClosedOnLegacyNotificationAndIgnoresUnknownExtensionNotification, TestPendingDeliveryAndCancellationClaimsRemainAtomic, TestHTTPToolHeaderCompilerAllowsOnlyTopLevelProperties, TestStreamableHTTPMissingOrNullArgumentsOmitToolParameterHeaders, TestHTTPToolHeaderBindingsReplaceAtomicallyAndRemoveStaleAuthority, TestMCPHeaderValueEncodingRejectsTabControlsAndSentinelAmbiguity, TestSSEProvenanceRejectsCrossRequestAndPreAcknowledgementNotifications, TestStreamableHTTPCorrelatedBadRequestPreservesRPCError, TestStreamableHTTPNon2xxAcceptsOnlyCorrelatedRPCErrorMatrix, TestHTTPStatusRPCSemanticsMatrix, TestReservedHTTP400ErrorsAreRejectedOnDirectAndSSEResponses, TestSubscriptionSSEAllowsTypedTerminalErrorBeforeAcknowledgement
- `transport_http_test.go` (baseline): TestStreamableHTTP_JSONVersionIgnoresSessionAndNeverDeletes, TestStreamableHTTP_POSTAcceptsSSEResponse, TestStreamableHTTP_ServerRequestFailsWithoutGETResumeOrReply, TestStreamableHTTP_MalformedIncomingRequestFailsWithoutReply, TestStreamableHTTP_HTTP404AndLegacyEndpointFailClosed
- `transport_resource_limits_test.go` (baseline): TestTransportLimitsRejectNegativeAndNilOptionsBeforeStart, TestSSEFrameBudgetIsIndependentOfOptionalLifetime, TestSSEFrameBudgetCountsMultipleDataLinesAndDelimiters, TestStdioTrafficCanExceedFrameBudgetAcrossMessages, TestPendingRequestBudgetReleasesOnTerminalPaths, TestTransportRegressionStdioCRLFFrameBudget, TestTransportRegressionStdioOutgoingNotificationTypedLimit, TestTransportAdmissionLimitIdentity
- `transport_sse_eof_migration_test.go` (baseline): TestMigratedSSEDiscardsUnfinishedTerminalAtEOF, TestMigratedSSEUnfinishedNotificationsDoNotDispatch
- `transport_sse_test.go` (baseline): TestSSEMigration_ConfiguredPostURL_BlocksPrivateIP, TestSSEMigration_ConfiguredPostURL_AllowPrivateIPs, TestSSEMigration_ValidateInitialURL_BlocksPrivateIP, TestSSEMigration_HTTPNotificationsForbiddenWith500Fixture, TestSSEMigration_Call_Non2xxPOSTStatus, TestSSEMigration_StartDoesNotGET500Endpoint, TestSSEMigration_StartDoesNotGET204Endpoint, TestSSEMigration_Start_PreCancelledWithoutEndpointWait, TestSSEMigration_Call_CancelUnblocksPending, TestSSEMigration_ExceedsMaxStreamBytes, TestSSEMigration_OriginalEndpointAndMalformedDataFramesFailClosed, TestSSEMigration_ResultBoundary_CancelOverReadLimit, TestSSEMigration_ResultBoundary_CancelOverStaleStream, TestSSEMigration_ResultBoundary_LimitWithoutCancel, TestSSEMigration_ResultBoundary_InterruptInChainOverReadLimit, TestSSEMigration_TerminalInterruptOverReadLimit, TestSSEMigration_StreamLimitPrecedesForbiddenEndpoint, TestSSEMigration_PreCancelledPrepareOverTerminalLimit, TestSSEMigration_PreCancelledCallOverTerminalLimit, TestSSEMigration_ResultBoundary_DeadlineOverReadLimit
- `transport_stdio_contract_test.go` (baseline): TestStdio_StderrVolumeAndLongLinesNeverCrashTransport, TestStdio_RequestDrivenServerReceivesDiscoveryBeforeWriting, TestStdio_ProcessExitUnblocksPendingRequest, TestStdio_NonzeroProcessExitWinsStdoutEOFRace, TestStdio_ClosedStdoutTerminatesAndReapsLiveProcess, TestStdio_BlockedWriteHonorsContextCancellation, TestStdio_CancellingQueuedWriteDoesNotAbortActiveWrite, TestStdio_WriteLoopRechecksCancellationAfterActivation, TestStdio_SuccessfulWriteIsFinishedBeforeDeliveryCallback, TestStdio_InvalidUTF8FailsClosedWithoutReply, TestStdio_MalformedCorrelatedResponseUnblocksPendingRequest, TestStdio_ProcessExitDetectedWhileDescendantHoldsStdout, TestStdio_ProcessExitDetectedWhileDescendantHoldsStderr, TestStdio_CloseKillsDescendantProcessTree, TestStdio_MalformedIncomingRequestFailsClosedWithoutReply, TestStdioHelperProcess
- `transport_stdio_test.go` (baseline): TestStdioTransport_Start_CancelContext, TestStdioTransport_StderrLongLineDoesNotBreakStart, TestStdioTransport_StdoutExceedsMaxStreamBytes, TestStdioTransport_PreparedCancellationUnblocksPending, TestStdioTransport_PrepareAfterStartContextCanceled, TestStdioTransport_CallerCancellationOverReadLimit, TestStdioTransport_PreCancelledPrepareOverTerminalReadLimit, TestStdioTransport_CallerCancellationOverStaleStream, TestStdioTransport_ReadLimitWithoutCancellation, TestStdioTransport_InterruptInChainOverReadLimit, TestStdioTransport_TerminalInterruptOverReadLimit
