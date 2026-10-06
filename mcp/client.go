//nolint:exhaustruct_v5 // Wire DTO constructors intentionally spell only meaningful fields.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"slices"
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
	discoverResultSubject     = MethodServerDiscover + " result"
	requestParamsSubject      = "request params"
	requestLogSubject         = "request log correlation"
	mcpLogLevelInfo           = "info"
	mcpLogLevelDebug          = "debug"
	mcpLogLevelNotice         = "notice"
	mcpLogLevelWarning        = "warning"
	mcpLogLevelAlert          = "alert"
	mcpLogLevelCritical       = "critical"
	mcpLogLevelEmergency      = "emergency"
	mcpLogLevelError          = "error"
	toolInputSchemaSubject    = "tool inputSchema"
	toolsListResultSubject    = "tools/list result"
	toolOutputSchemaSubject   = "tool outputSchema"
	defaultClientVersion      = "1.0.0"
)

type ClientOption func(*ClientOptions)

type ClientOptions struct {
	Logger           *slog.Logger
	ClientInfo       Implementation
	Pagination       PaginationLimits
	Subscriptions    SubscriptionLimits
	ToolPolicyMapper ToolPolicyMapper
}

func WithClientLogger(logger *slog.Logger) ClientOption {
	return func(options *ClientOptions) { options.Logger = logger }
}

func WithClientInfo(info Implementation) ClientOption {
	snapshot := cloneImplementation(info)
	return func(options *ClientOptions) { options.ClientInfo = cloneImplementation(snapshot) }
}

func WithPaginationLimits(limits PaginationLimits) ClientOption {
	return func(options *ClientOptions) { options.Pagination = limits }
}

type InvalidationKind string

const (
	InvalidationTools     InvalidationKind = "tools"
	InvalidationResources InvalidationKind = "resources"
	InvalidationResource  InvalidationKind = "resource"
	InvalidationPrompts   InvalidationKind = "prompts"
)

// Invalidation carries the authoritative generation and subscription provenance.
type Invalidation struct {
	Kind           InvalidationKind
	URI            string
	Generation     uint64
	SubscriptionID string
}

// InputRequiredError exposes a non-terminal MCP MRTR result. The client never
// retries it automatically; the host may explicitly issue a new request.
type InputRequiredError struct {
	Method string
	Result InputRequiredResult
}

func (e *InputRequiredError) Error() string {
	return fmt.Sprintf("mcp: %s requires additional input", e.Method)
}

// Subscription represents one explicit subscriptions/listen operation.
type Subscription struct {
	ID     string
	Events <-chan Invalidation
	done   <-chan struct{}
	err    func() error
	cancel context.CancelFunc
}

func (s *Subscription) Done() <-chan struct{} { return s.done }
func (s *Subscription) Err() error            { return s.err() }
func (s *Subscription) Close()                { s.cancel() }

type subscriptionState struct {
	key           string
	publicID      string
	requested     SubscriptionFilter
	effective     SubscriptionFilter
	acked         bool
	closed        bool
	events        chan Invalidation
	done          chan struct{}
	cancel        context.CancelFunc
	mu            sync.RWMutex
	err           error
	ctx           context.Context
	localCanceled bool
}

type progressState struct {
	mu      sync.Mutex
	last    float64
	started bool
	retired bool
	ch      chan<- toolsy.Chunk
	done    <-chan struct{}
}

type Client struct {
	transport Transport
	opts      ClientOptions
	logger    *slog.Logger

	mu                   sync.RWMutex
	ready                bool
	closed               bool
	server               DiscoverResult
	subscriptions        map[string]*subscriptionState
	retiredSubscriptions map[string]time.Time
	toolBindings         map[string]struct{}
	toolSchemas          map[string]schemaValidator
	toolBindingMu        sync.Mutex

	progressCounter    atomic.Uint64
	progressCallbacks  sync.Map
	invalidationCount  atomic.Uint64
	toolGeneration     atomic.Uint64
	resourceGeneration atomic.Uint64
	promptGeneration   atomic.Uint64
	requestLogs        sync.Map
	invalidations      chan Invalidation
	closeOnce          sync.Once
	closeErr           error
}

func Connect(ctx context.Context, transport Transport, opts ...ClientOption) (*Client, error) {
	options := ClientOptions{
		ClientInfo: Implementation{Name: "toolsy-mcp-client", Version: defaultClientVersion},
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("mcp: nil client option")
		}
		opt(&options)
	}
	if err := options.Pagination.validate(); err != nil {
		return nil, err
	}
	options.Pagination = options.Pagination.normalized()
	var limitsErr error
	options.Subscriptions, limitsErr = options.Subscriptions.normalized()
	if limitsErr != nil {
		return nil, limitsErr
	}
	// ClientOption is intentionally open for composition. Re-snapshot the final
	// value so a custom option cannot retain mutable metadata aliases.
	options.ClientInfo = cloneImplementation(options.ClientInfo)
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if err := validateImplementation(options.ClientInfo); err != nil {
		return nil, &InvalidPayloadError{Subject: "clientInfo", Err: err}
	}
	client := &Client{
		transport:            transport,
		opts:                 options,
		logger:               options.Logger,
		subscriptions:        make(map[string]*subscriptionState),
		retiredSubscriptions: make(map[string]time.Time),
		toolBindings:         make(map[string]struct{}),
		toolSchemas:          make(map[string]schemaValidator),
		invalidations:        make(chan Invalidation, clientEventBufferSize),
	}
	if err := transport.Start(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}
	client.registerHandlers()
	if err := client.discover(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}
	return client, nil
}

func validateImplementation(info Implementation) error {
	if info.Name == "" || info.Version == "" {
		return errors.New("name and version are required")
	}
	return validateIcons(info.Icons)
}

func cloneImplementation(info Implementation) Implementation {
	info.Extra = cloneMeta(info.Extra)
	info.Icons = append([]Icon(nil), info.Icons...)
	for index := range info.Icons {
		info.Icons[index].Sizes = append([]string(nil), info.Icons[index].Sizes...)
		info.Icons[index].Extra = cloneMeta(info.Icons[index].Extra)
	}
	return info
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

// clientCapabilities intentionally advertises no optional server-to-client
// capabilities. MRTR requestState continuation is handled by the host, while
// inputRequests requiring elicitation/sampling are rejected because this
// bridge implements neither feature. Request-scoped logging is opted into per
// request through logLevel and therefore has no discovery capability bit.
func (c *Client) clientCapabilities() ClientCapabilities { return ClientCapabilities{} }

// prepareParams is the sole producer of self-describing request metadata.
//
//nolint:nestif // One reserved-key ownership boundary keeps caller metadata validation atomic.
func (c *Client) prepareParams(params any) (json.RawMessage, error) {
	params, callerMeta := detachRequestMeta(params)
	info := cloneImplementation(c.opts.ClientInfo)
	meta := &RequestMeta{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: c.clientCapabilities(),
		ClientInfo:         &info,
	}
	if callerMeta != nil {
		if callerMeta.ProtocolVersion != "" || callerMeta.ClientInfo != nil ||
			hasClientCapabilityDeclarations(callerMeta.ClientCapabilities) {
			return nil, &InvalidPayloadError{
				Subject: "request _meta",
				Err:     errors.New("caller cannot set core-owned request identity"),
			}
		}
		for key, value := range callerMeta.Extra {
			if strings.HasPrefix(key, "io.modelcontextprotocol/") || key == progressTokenField {
				return nil, &InvalidPayloadError{
					Subject: "request _meta",
					Err:     fmt.Errorf("caller cannot set reserved key %q", key),
				}
			}
			if meta.Extra == nil {
				meta.Extra = make(Meta)
			}
			meta.Extra[key] = bytes.Clone(value)
		}
		meta.ProgressToken = callerMeta.ProgressToken
		if callerMeta.LogLevel != "" {
			if _, ok := mcpLogLevel(callerMeta.LogLevel); !ok {
				return nil, &InvalidPayloadError{
					Subject: "request log level",
					Err:     fmt.Errorf("unsupported level %q", callerMeta.LogLevel),
				}
			}
			meta.LogLevel = callerMeta.LogLevel
		}
	}
	params, err := attachRequestMeta(params, meta)
	if err != nil {
		return nil, err
	}
	return json.Marshal(params)
}

func hasClientCapabilityDeclarations(capabilities ClientCapabilities) bool {
	return len(capabilities.Experimental) != 0 ||
		capabilities.Roots != nil ||
		capabilities.Sampling != nil ||
		capabilities.Elicitation != nil ||
		len(capabilities.Extensions) != 0 ||
		len(capabilities.Extra) != 0
}

func attachRequestMeta(params any, meta *RequestMeta) (any, error) {
	value := reflect.ValueOf(params)
	wasPointer := value.IsValid() && value.Kind() == reflect.Pointer
	if wasPointer {
		if value.IsNil() {
			return nil, &InvalidPayloadError{Subject: requestParamsSubject, Err: errors.New("params must not be nil")}
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil, &InvalidPayloadError{
			Subject: requestParamsSubject,
			Err:     errors.New("params must be a struct with *RequestMeta field"),
		}
	}
	copyValue := reflect.New(value.Type()).Elem()
	copyValue.Set(value)
	field := copyValue.FieldByName("Meta")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeFor[*RequestMeta]() {
		return nil, &InvalidPayloadError{
			Subject: requestParamsSubject,
			Err:     errors.New("params must contain a writable *RequestMeta field"),
		}
	}
	field.Set(reflect.ValueOf(meta))
	if wasPointer {
		pointer := reflect.New(copyValue.Type())
		pointer.Elem().Set(copyValue)
		return pointer.Interface(), nil
	}
	return copyValue.Interface(), nil
}

// detachRequestMeta shallow-copies a params struct and removes its Meta field
// before JSON marshaling. RequestMeta itself is only valid after core-owned
// reserved fields have been injected by prepareParams.
func detachRequestMeta(params any) (any, *RequestMeta) {
	value := reflect.ValueOf(params)
	wasPointer := value.IsValid() && value.Kind() == reflect.Pointer
	if wasPointer {
		if value.IsNil() {
			return params, nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return params, nil
	}
	copyValue := reflect.New(value.Type()).Elem()
	copyValue.Set(value)
	field := copyValue.FieldByName("Meta")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeFor[*RequestMeta]() {
		return params, nil
	}
	var meta *RequestMeta
	if !field.IsNil() {
		original, ok := reflect.TypeAssert[*RequestMeta](field)
		if !ok {
			return params, nil
		}
		cloned := *original
		cloned.Extra = cloneMeta(original.Extra)
		meta = &cloned
	}
	field.Set(reflect.Zero(field.Type()))
	if wasPointer {
		pointer := reflect.New(copyValue.Type())
		pointer.Elem().Set(copyValue)
		return pointer.Interface(), meta
	}
	return copyValue.Interface(), meta
}

func cloneMeta(meta Meta) Meta {
	if meta == nil {
		return nil
	}
	cloned := make(Meta, len(meta))
	for key, value := range meta {
		cloned[key] = bytes.Clone(value)
	}
	return cloned
}

func (c *Client) discover(ctx context.Context) error {
	raw, err := c.requestAndAwait(ctx, MethodServerDiscover, DiscoverParams{})
	if err != nil {
		return c.mapCallReadLimitFor(ctx, err, "MCP server discovery response")
	}
	var result DiscoverResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return &InvalidPayloadError{Subject: discoverResultSubject, Err: err}
	}
	seen := make(map[string]struct{}, len(result.SupportedVersions))
	supported := false
	for _, version := range result.SupportedVersions {
		if version == "" {
			return &InvalidPayloadError{
				Subject: discoverResultSubject,
				Err:     errors.New("supportedVersions contains an empty version"),
			}
		}
		if _, duplicate := seen[version]; duplicate {
			return &InvalidPayloadError{
				Subject: discoverResultSubject,
				Err:     fmt.Errorf("duplicate supported version %q", version),
			}
		}
		seen[version] = struct{}{}
		supported = supported || version == ProtocolVersion
	}
	if !supported {
		return &ProtocolVersionError{Requested: ProtocolVersion, Selected: strings.Join(result.SupportedVersions, ",")}
	}
	if result.Meta.ServerInfo != nil {
		if err := validateImplementation(*result.Meta.ServerInfo); err != nil {
			return &InvalidPayloadError{Subject: "serverInfo", Err: err}
		}
	}
	c.mu.Lock()
	c.server = result
	c.ready = true
	c.mu.Unlock()
	return nil
}

func (c *Client) registerHandlers() {
	c.transport.OnNotification(MethodProgress, c.handleProgress)
	if contextual, ok := c.transport.(RequestScopedNotificationTransport); ok {
		contextual.OnRequestNotification(MethodLogMessage, c.handleRequestLogMessage)
	}
	c.transport.OnNotification(MethodSubscriptionsAcknowledged, c.handleSubscriptionAcknowledged)
	c.transport.OnNotification(
		MethodToolsListChanged,
		func(raw json.RawMessage) { c.handleSubscriptionInvalidation(InvalidationTools, raw) },
	)
	c.transport.OnNotification(
		MethodResourcesListChanged,
		func(raw json.RawMessage) { c.handleSubscriptionInvalidation(InvalidationResources, raw) },
	)
	c.transport.OnNotification(
		MethodPromptsListChanged,
		func(raw json.RawMessage) { c.handleSubscriptionInvalidation(InvalidationPrompts, raw) },
	)
	c.transport.OnNotification(
		MethodResourceUpdated,
		func(raw json.RawMessage) { c.handleSubscriptionInvalidation(InvalidationResource, raw) },
	)
}

func mcpLogLevel(level string) (slog.Level, bool) {
	switch level {
	case mcpLogLevelDebug:
		return slog.LevelDebug, true
	case mcpLogLevelInfo, mcpLogLevelNotice:
		return slog.LevelInfo, true
	case mcpLogLevelWarning:
		return slog.LevelWarn, true
	case mcpLogLevelError, mcpLogLevelCritical, mcpLogLevelAlert, mcpLogLevelEmergency:
		return slog.LevelError, true
	default:
		return 0, false
	}
}

func (c *Client) handleRequestLogMessage(requestID, raw json.RawMessage) {
	key, err := rpcIDKey(requestID)
	if err != nil {
		return
	}
	if _, active := c.requestLogs.Load("r:" + key); !active {
		return
	}
	message, ok := c.validLogMessage(raw)
	if !ok {
		return
	}
	c.emitLogMessage(message)
}

func (c *Client) validLogMessage(raw json.RawMessage) (LogMessageParams, bool) {
	var message LogMessageParams
	if err := json.Unmarshal(raw, &message); err != nil {
		c.logger.Warn("mcp: invalid request-scoped log notification", "err", err)
		return LogMessageParams{}, false
	}
	_, ok := mcpLogLevel(message.Level)
	if !ok || len(message.Data) == 0 || !json.Valid(message.Data) {
		c.logger.Warn("mcp: invalid request-scoped log notification")
		return LogMessageParams{}, false
	}
	return message, true
}

func (c *Client) emitLogMessage(message LogMessageParams) {
	level, _ := mcpLogLevel(message.Level)
	c.logger.Log(
		context.Background(),
		level,
		"mcp: request-scoped log",
		"logger", boundedDiagnostic(message.Logger),
		"data", boundedDiagnostic(string(message.Data)),
	)
}

func (c *Client) requireCapability(name string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.ready {
		return &CapabilityError{Capability: MethodServerDiscover}
	}
	switch name {
	case string(InvalidationTools):
		if c.server.Capabilities.Tools == nil {
			return &CapabilityError{Capability: name}
		}
	case string(InvalidationResources):
		if c.server.Capabilities.Resources == nil {
			return &CapabilityError{Capability: name}
		}
	case string(InvalidationPrompts):
		if c.server.Capabilities.Prompts == nil {
			return &CapabilityError{Capability: name}
		}
	}
	return nil
}

type requestLogScope struct {
	enabled bool
}

func requestLogCorrelation(prepared json.RawMessage) (requestLogScope, error) {
	params, err := decodeObjectFields(prepared)
	if err != nil {
		return requestLogScope{}, err
	}
	meta, err := decodeObjectFields(params["_meta"])
	if err != nil {
		return requestLogScope{}, err
	}
	if _, logging := meta[metaLogLevel]; !logging {
		return requestLogScope{}, nil
	}
	return requestLogScope{enabled: true}, nil
}

func (c *Client) prepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, requestLogScope, error) {
	prepared, err := c.prepareParams(params)
	if err != nil {
		return nil, requestLogScope{}, err
	}
	logScope, err := requestLogCorrelation(prepared)
	if err != nil {
		return nil, requestLogScope{}, &InvalidPayloadError{Subject: requestLogSubject, Err: err}
	}
	if logScope.enabled {
		if _, contextual := c.transport.(RequestScopedNotificationTransport); !contextual {
			return nil, requestLogScope{}, &UnsupportedFeatureError{
				Feature: "request-scoped logging on a shared transport",
			}
		}
	}
	pending, err := c.transport.PrepareRequest(ctx, method, prepared)
	return pending, logScope, err
}

func (c *Client) deliverPrepared(pending PreparedRequest, scope requestLogScope) error {
	if !scope.enabled {
		return pending.Deliver()
	}
	requestKey, err := rpcIDKey(pending.ID())
	if err != nil {
		_ = pending.Abort(err)
		return &InvalidPayloadError{Subject: requestLogSubject, Err: err}
	}
	logKey := "r:" + requestKey
	completion, ok := pending.(CompletionPendingRequest)
	if !ok {
		_ = pending.Abort(errors.New("request-scoped logging lifecycle is unavailable"))
		return &UnsupportedFeatureError{Feature: "request-scoped logging lifecycle"}
	}
	state := &struct{}{}
	if _, loaded := c.requestLogs.LoadOrStore(logKey, state); loaded {
		err := &InvalidPayloadError{
			Subject: requestLogSubject,
			Err:     errors.New("correlation token is already active"),
		}
		_ = pending.Abort(err)
		return err
	}
	completion.OnComplete(func() { c.requestLogs.CompareAndDelete(logKey, state) })
	if err := pending.Deliver(); err != nil {
		c.requestLogs.CompareAndDelete(logKey, state)
		return err
	}
	return nil
}

func (c *Client) request(ctx context.Context, method string, params any) (PendingRequest, error) {
	pending, logging, err := c.prepareRequest(ctx, method, params)
	if err != nil {
		return nil, err
	}
	if err := c.deliverPrepared(pending, logging); err != nil {
		return nil, err
	}
	return pending, nil
}

func (c *Client) requestAndAwait(ctx context.Context, method string, params any) (json.RawMessage, error) {
	pending, err := c.request(ctx, method, params)
	if err != nil {
		return nil, err
	}
	return c.awaitPending(ctx, pending)
}

func (c *Client) awaitPending(
	ctx context.Context,
	pending PendingRequest,
) (json.RawMessage, error) {
	result, err := pending.Await(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		c.notifyCancelledPending(ctx, pending, ctxErr.Error())
	}
	return result, mapTypedRPCError(err)
}

func mapTypedRPCError(err error) error {
	if rpcErr, ok := errors.AsType[*RPCError](err); ok {
		return TypedRPCError(rpcErr)
	}
	return err
}

func (c *Client) maxFrameBytes() int {
	if transport, ok := c.transport.(FrameByteCapTransport); ok && transport.MaxFrameBytes() > 0 {
		return transport.MaxFrameBytes()
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
		mapped := toolsy.MapReadLimitErrorFor(err, c.maxFrameBytes(), subject, "")
		if toolErr, ok := toolsy.AsToolError(mapped); ok {
			// Preserve diagnostic identity without exposing the transport cause in
			// the bounded validation reason or mutating a shared error instance.
			withCause := *toolErr
			withCause.Err = errors.Join(toolErr.Err, err)
			return &withCause
		}
		return mapped
	}
	return err
}

//nolint:nilnil // A nil result with nil error is the explicit terminal-complete discriminator.
func requireCompleteResult(raw json.RawMessage, method string, allowInput bool) (*InputRequiredResult, error) {
	fields, err := decodeObjectFields(raw)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: method + " result", Err: err}
	}
	var resultType string
	if err := json.Unmarshal(fields["resultType"], &resultType); err != nil || resultType == "" {
		return nil, &InvalidPayloadError{Subject: method + " result", Err: errors.New("resultType is required")}
	}
	switch resultType {
	case ResultTypeComplete:
		return nil, nil
	case ResultTypeInputRequired:
		if !allowInput {
			return nil, &InvalidPayloadError{
				Subject: method + " result",
				Err:     errors.New("input_required is not allowed for this method"),
			}
		}
		var result InputRequiredResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, &InvalidPayloadError{Subject: method + " input_required result", Err: err}
		}
		requiresCapability, err := inputRequestsRequireCapability(result.InputRequests)
		if err != nil {
			return nil, &InvalidPayloadError{Subject: method + " inputRequests", Err: err}
		}
		if requiresCapability {
			return nil, &UnsupportedFeatureError{
				Feature: method + " input_required requires undeclared elicitation or sampling capability",
			}
		}
		return &result, nil
	default:
		return nil, &InvalidPayloadError{
			Subject: method + " result",
			Err:     fmt.Errorf("unknown resultType %q", resultType),
		}
	}
}

func inputRequestsRequireCapability(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 || rawJSONIsNull(raw) {
		return false, nil
	}
	requests, err := decodeObjectFields(raw)
	if err != nil {
		return false, err
	}
	return len(requests) > 0, nil
}

func (c *Client) listToolsPage(
	ctx context.Context,
	cursor string,
	generation uint64,
	budget *toolDiscoveryBudget,
) (ToolsListResult, []toolBindingCandidate, error) {
	raw, err := c.requestAndAwait(ctx, MethodToolsList, ToolsListParams{Cursor: cursor})
	if err != nil {
		return ToolsListResult{}, nil, c.mapCallReadLimitFor(ctx, err, "MCP tools list response")
	}
	if budgetErr := budget.accept(raw); budgetErr != nil {
		return ToolsListResult{}, nil, budgetErr
	}
	if _, completeErr := requireCompleteResult(raw, MethodToolsList, false); completeErr != nil {
		return ToolsListResult{}, nil, completeErr
	}
	var result ToolsListResult
	if unmarshalErr := json.Unmarshal(raw, &result); unmarshalErr != nil {
		return ToolsListResult{}, nil, &InvalidPayloadError{Subject: toolsListResultSubject, Err: unmarshalErr}
	}
	if current := c.toolGeneration.Load(); current != generation {
		return ToolsListResult{}, nil, staleError(InvalidationTools, generation, current)
	}
	filtered, candidates, err := c.prepareToolCandidates(ctx, result.Tools)
	if err != nil {
		return ToolsListResult{}, nil, err
	}
	result.Tools = filtered
	return result, candidates, nil
}

type toolBindingCandidate struct {
	descriptor MCPTool
	bindings   []HTTPToolHeaderBinding
	validator  schemaValidator
}

func (c *Client) prepareToolCandidates(
	ctx context.Context,
	descriptors []MCPTool,
) ([]MCPTool, []toolBindingCandidate, error) {
	candidates := make([]toolBindingCandidate, 0, len(descriptors))
	seen := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		if _, duplicate := seen[descriptor.Name]; duplicate {
			return nil, nil, &InvalidPayloadError{
				Subject: toolsListResultSubject,
				Err:     fmt.Errorf("duplicate tool name %q", descriptor.Name),
			}
		}
		seen[descriptor.Name] = struct{}{}
		if err := validateMCPTool(descriptor); err != nil {
			return nil, nil, &InvalidPayloadError{Subject: "tool descriptor", Err: err}
		}
		if !mcpToolNamePattern.MatchString(descriptor.Name) {
			return nil, nil, &InvalidPayloadError{
				Subject: "tool name",
				Err:     fmt.Errorf("invalid name %q", descriptor.Name),
			}
		}
		inputSchema, inputErr := decodeSchemaObject(descriptor.InputSchema, true)
		if inputErr != nil {
			return nil, nil, &InvalidPayloadError{Subject: toolInputSchemaSubject, Err: inputErr}
		}
		if _, err := jsonschemax.Compile(inputSchema); err != nil {
			return nil, nil, &InvalidPayloadError{Subject: toolInputSchemaSubject, Err: err}
		}
		if _, err := decodeSchemaObject(descriptor.OutputSchema, false); err != nil {
			return nil, nil, &InvalidPayloadError{Subject: toolOutputSchemaSubject, Err: err}
		}
		bindings, err := compileHTTPToolHeaderBindings(descriptor.InputSchema)
		if err != nil {
			c.logger.WarnContext(
				ctx,
				"mcp: excluding tool with invalid x-mcp-header",
				"tool", boundedDiagnostic(descriptor.Name),
				"err", err,
			)
			continue
		}
		validator, err := compileOutputSchema(descriptor.OutputSchema)
		if err != nil {
			return nil, nil, &InvalidPayloadError{Subject: toolOutputSchemaSubject, Err: err}
		}
		candidates = append(candidates, toolBindingCandidate{
			descriptor: descriptor,
			bindings:   bindings,
			validator:  validator,
		})
	}
	filtered := make([]MCPTool, len(candidates))
	for index := range candidates {
		filtered[index] = candidates[index].descriptor
	}
	return filtered, candidates, nil
}

func (c *Client) publishToolAuthority(ctx context.Context, candidates []toolBindingCandidate, generation uint64) error {
	bindings := make(map[string][]HTTPToolHeaderBinding, len(candidates))
	schemas := make(map[string]schemaValidator, len(candidates))
	names := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		bindings[candidate.descriptor.Name] = cloneHTTPToolHeaderBindings(candidate.bindings)
		schemas[candidate.descriptor.Name] = candidate.validator
		names[candidate.descriptor.Name] = struct{}{}
	}
	c.toolBindingMu.Lock()
	defer c.toolBindingMu.Unlock()
	// Publication commits after this cancellation check. Once the trusted facet
	// starts replacing bindings, cancellation cannot undo a successful commit.
	if err := ctx.Err(); err != nil {
		return err
	}
	if current := c.toolGeneration.Load(); current != generation {
		return staleError(InvalidationTools, generation, current)
	}
	if headerTransport, ok := c.transport.(ToolHeaderTransport); ok {
		if err := headerTransport.ReplaceToolHeaderBindings(bindings); err != nil {
			return err
		}
	}
	if current := c.toolGeneration.Load(); current != generation {
		if headerTransport, ok := c.transport.(ToolHeaderTransport); ok {
			_ = headerTransport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{})
		}
		clear(c.toolBindings)
		clear(c.toolSchemas)
		return staleError(InvalidationTools, generation, current)
	}
	c.toolBindings = names
	c.toolSchemas = schemas
	return nil
}

func (c *Client) toolAuthority(name string) (schemaValidator, error) {
	_, needsAuthority := c.transport.(ToolHeaderTransport)
	c.toolBindingMu.Lock()
	_, known := c.toolBindings[name]
	validator := c.toolSchemas[name]
	c.toolBindingMu.Unlock()
	if needsAuthority && !known {
		return nil, &InvalidPayloadError{
			Subject: "tools/call routing authority",
			Err:     fmt.Errorf("tool %q has no current authoritative descriptor", name),
		}
	}
	return validator, nil
}

// ListToolsPage returns one validated page without changing routing authority.
// Use DiscoverTools for a complete transactional authority snapshot.
func (c *Client) ListToolsPage(ctx context.Context, cursor string) (ToolsListResult, error) {
	if err := c.requireCapability("tools"); err != nil {
		return ToolsListResult{}, err
	}
	budget := newToolDiscoveryBudget(c.opts.Pagination)
	result, _, err := c.listToolsPage(ctx, cursor, c.toolGeneration.Load(), &budget)
	return result, err
}

// DiscoverTools fetches and validates a complete typed snapshot, then publishes
// routing authority atomically. Page-specific cache hints remain in Pages.
func (c *Client) DiscoverTools(ctx context.Context) (ToolDiscovery, error) {
	snapshot, _, err := c.discoverToolCandidates(ctx)
	return snapshot, err
}

// Discover builds tool proxies from the same full validated discovery path.
func (c *Client) Discover(ctx context.Context) iter.Seq2[toolsy.Tool, error] {
	return func(yield func(toolsy.Tool, error) bool) {
		snapshot, candidates, err := c.discoverToolCandidates(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, candidate := range candidates {
			proxy, err := c.toolToProxyAtGeneration(ctx, candidate.descriptor, snapshot.Generation)
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
	return func(yield func(T, error) bool) { var zero T; yield(zero, err) }
}

func staleError(kind InvalidationKind, discovered, current uint64) error {
	return &StaleDiscoveryError{Kind: kind, Discovered: discovered, Current: current}
}

var mcpToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func (c *Client) toolToProxyAtGeneration(
	ctx context.Context,
	descriptor MCPTool,
	generation uint64,
) (toolsy.Tool, error) {
	if err := validateMCPTool(descriptor); err != nil {
		return nil, &InvalidPayloadError{Subject: "tool descriptor", Err: err}
	}
	if !mcpToolNamePattern.MatchString(descriptor.Name) {
		return nil, &InvalidPayloadError{Subject: "tool name", Err: fmt.Errorf("invalid name %q", descriptor.Name)}
	}
	inputSchema, err := decodeSchemaObject(descriptor.InputSchema, true)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: toolInputSchemaSubject, Err: err}
	}
	outputSchema, err := decodeSchemaObject(descriptor.OutputSchema, false)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: toolOutputSchemaSubject, Err: err}
	}
	validator, err := compileOutputSchema(descriptor.OutputSchema)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: toolOutputSchemaSubject, Err: err}
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
		if current := c.toolGeneration.Load(); current != generation {
			return staleError(InvalidationTools, generation, current)
		}
		return c.runMCPToolCall(ctx, descriptor.Name, rawArgs, validator, generation, yield)
	}
	options, err := c.toolPolicyOptions(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if outputSchema != nil {
		options = append(options, toolsy.WithOutputSchema(outputSchema))
	}
	base, err := toolsy.NewProxyTool(descriptor.Name, description, schemaJSON, handler, options...)
	if err != nil {
		return nil, err
	}
	return &generationTool{Tool: base, client: c, generation: generation}, nil
}

func validateMCPTool(descriptor MCPTool) error {
	if descriptor.Name == "" {
		return errors.New("name is required")
	}
	return validateIcons(descriptor.Icons)
}
func validatePrompt(prompt Prompt) error { return validateIcons(prompt.Icons) }

//nolint:nilnil // Optional schemas deliberately decode to nil without an error.
func decodeSchemaObject(raw json.RawMessage, required bool) (map[string]any, error) {
	if len(raw) == 0 || rawJSONIsNull(raw) {
		if required {
			return nil, errors.New("schema is required and must not be null")
		}
		return nil, nil
	}
	decoded, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	schema, ok := decoded.(map[string]any)
	if !ok || schema == nil {
		return nil, errors.New("schema must be a JSON object")
	}
	if required {
		if schemaType, ok := schema["type"]; !ok || schemaType != schemaTypeObject {
			return nil, errors.New("input schema root type must be object")
		}
	}
	if _, err := jsonschemax.Compile(schema); err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

type schemaValidator interface{ Validate(any) error }

//nolint:nilnil // An absent output schema deliberately means no validator.
func compileOutputSchema(raw json.RawMessage) (schemaValidator, error) {
	if len(raw) == 0 || rawJSONIsNull(raw) {
		return nil, nil
	}
	schema, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	return jsonschemax.Compile(schema)
}

type toolCallFinalResult struct {
	raw json.RawMessage
	err error
}

//nolint:gocognit // Explicit ordering avoids cancellation/progress races.
func (c *Client) runMCPToolCall(
	ctx context.Context,
	name string,
	rawArgs []byte,
	outputSchema schemaValidator,
	generation uint64,
	yield func(toolsy.Chunk) error,
) error {
	if _, err := c.toolAuthority(name); err != nil {
		return err
	}
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	token := NewStringProgressToken(c.nextProgressToken())
	tokenKey, err := progressTokenKey(token)
	if err != nil {
		return err
	}
	progressCh := make(chan toolsy.Chunk, progressChunkBufferSize)
	done := make(chan struct{})
	state := &progressState{ch: progressCh, done: done}
	c.progressCallbacks.Store(tokenKey, state)
	defer func() { c.progressCallbacks.Delete(tokenKey); close(done) }()
	pending, logging, err := c.prepareRequest(
		callCtx,
		MethodToolsCall,
		ToolsCallParams{
			Name: name, Arguments: bytes.Clone(rawArgs),
			Meta: &RequestMeta{ProgressToken: token},
		},
	)
	if err != nil {
		return err
	}
	completion, ok := pending.(CompletionPendingRequest)
	if !ok {
		cause := &UnsupportedFeatureError{Feature: "request-scoped progress lifecycle"}
		_ = pending.Abort(cause)
		return cause
	}
	completion.OnComplete(func() {
		state.mu.Lock()
		state.retired = true
		c.progressCallbacks.CompareAndDelete(tokenKey, state)
		state.mu.Unlock()
	})
	notifyConsumerAbort := func() {
		state.mu.Lock()
		retired := state.retired
		state.mu.Unlock()
		if retired && ctx.Err() == nil {
			return
		}
		c.notifyCancelledPending(ctx, pending, "result consumer aborted")
	}
	if err := c.deliverPrepared(pending, logging); err != nil {
		return err
	}
	finalCh := make(chan toolCallFinalResult, 1)
	go func() {
		// Only the invocation loop owns cancellation notifications. Cancelling
		// callCtx on return must release custom Await without a second send.
		raw, awaitErr := pending.Await(callCtx)
		finalCh <- toolCallFinalResult{raw: raw, err: awaitErr}
	}()
	for {
		select {
		case <-ctx.Done():
			c.notifyCancelledPending(ctx, pending, ctx.Err().Error())
			return ctx.Err()
		case progress := <-progressCh:
			if err := yield(progress); err != nil {
				notifyConsumerAbort()
				return fmt.Errorf("%w: %w", toolsy.ErrStreamAborted, err)
			}
		case final := <-finalCh:
			if ctxErr := ctx.Err(); ctxErr != nil {
				c.notifyCancelledPending(ctx, pending, ctxErr.Error())
				return ctxErr
			}
			// Completion has retired the route and serialized with enqueueing.
			// Deliver every accepted pre-terminal notification before the result.
			for len(progressCh) > 0 {
				if err := yield(<-progressCh); err != nil {
					notifyConsumerAbort()
					return fmt.Errorf("%w: %w", toolsy.ErrStreamAborted, err)
				}
			}
			if final.err != nil {
				return c.mapCallReadLimitFor(ctx, mapTypedRPCError(final.err), "MCP tool response")
			}
			chunk, err := c.completeToolCallChunk(name, final.raw, outputSchema, generation)
			if err != nil {
				return err
			}
			if err := yield(chunk); err != nil {
				notifyConsumerAbort()
				return fmt.Errorf("%w: %w", toolsy.ErrStreamAborted, err)
			}
			return nil
		}
	}
}

func (c *Client) completeToolCallChunk(
	name string,
	raw json.RawMessage,
	outputSchema schemaValidator,
	generation uint64,
) (toolsy.Chunk, error) {
	if current := c.toolGeneration.Load(); current != generation {
		return toolsy.Chunk{}, staleError(InvalidationTools, generation, current)
	}
	input, err := requireCompleteResult(raw, MethodToolsCall, true)
	if err != nil {
		return toolsy.Chunk{}, err
	}
	if input != nil {
		return toolsy.Chunk{}, &InputRequiredError{Method: MethodToolsCall, Result: *input}
	}
	return buildToolResultChunk(name, raw, outputSchema)
}

func progressTokenKey(token ProgressToken) (string, error) {
	raw, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case string:
		return "s:" + typed, nil
	case json.Number:
		return "n:" + typed.String(), nil
	default:
		return "", errors.New("progress token must be a string or number")
	}
}

// CallTool executes one explicit MRTR round. A returned input-required result
// is not retried; the host can populate InputResponses/RequestState and call
// this method again, which always allocates a new JSON-RPC request ID.
//

func (c *Client) CallTool(ctx context.Context, params ToolsCallParams) (*CallToolResult, *InputRequiredResult, error) {
	if err := validateOutboundInputResponses(MethodToolsCall, params.InputResponses); err != nil {
		return nil, nil, err
	}
	if err := c.requireCapability("tools"); err != nil {
		return nil, nil, err
	}
	outputValidator, err := c.toolAuthority(params.Name)
	if err != nil {
		return nil, nil, err
	}
	generation := c.toolGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodToolsCall, params)
	if err != nil {
		return nil, nil, c.mapCallReadLimitFor(ctx, err, "MCP tool response")
	}
	if current := c.toolGeneration.Load(); current != generation {
		return nil, nil, staleError(InvalidationTools, generation, current)
	}
	input, err := requireCompleteResult(raw, MethodToolsCall, true)
	if err != nil || input != nil {
		return nil, input, err
	}
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, &InvalidPayloadError{Subject: "tools/call result", Err: err}
	}
	if _, err := formatContentBlocks(result.Content); err != nil {
		return nil, nil, &InvalidPayloadError{Subject: "tool content", Err: err}
	}
	if err := validatePresentStructuredContent(result.StructuredContent, outputValidator); err != nil {
		return nil, nil, err
	}
	if result.IsError {
		return &result, nil, &RemoteToolError{ToolName: params.Name, Result: result}
	}
	if outputValidator != nil && len(result.StructuredContent) == 0 {
		return nil, nil, missingStructuredContentError()
	}
	return &result, nil, nil
}

func buildToolResultChunk(name string, raw json.RawMessage, outputSchema schemaValidator) (toolsy.Chunk, error) {
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return toolsy.Chunk{}, &InvalidPayloadError{Subject: "tools/call result", Err: err}
	}
	projection, err := formatContentBlocks(result.Content)
	if err != nil {
		return toolsy.Chunk{}, &InvalidPayloadError{Subject: "tool content", Err: err}
	}
	if err := validatePresentStructuredContent(result.StructuredContent, outputSchema); err != nil {
		return toolsy.Chunk{}, err
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
		return toolsy.Chunk{}, missingStructuredContentError()
	}
	if len(result.StructuredContent) > 0 {
		canonical, value, err := canonicalJSON(result.StructuredContent)
		if err != nil {
			return toolsy.Chunk{}, &InvalidPayloadError{Subject: structuredContentSubject, Err: err}
		}
		if outputSchema != nil {
			if err := outputSchema.Validate(value); err != nil {
				return toolsy.Chunk{}, &InvalidPayloadError{Subject: structuredContentSubject, Err: err}
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
	mimeType := toolsy.MimeTypeText
	if len(projection) == 0 {
		mimeType = ""
	}
	return toolsy.Chunk{
		Event:       toolsy.EventResult,
		Data:        projection,
		MimeType:    mimeType,
		TypedResult: result,
		EmptyResult: len(projection) == 0,
		Envelope: toolsy.NewResultEnvelope(
			result,
			projection,
			mimeType,
			toolsy.DeliveryClassText,
			toolsy.AudienceModel,
			nil,
		),
	}, nil
}

func validatePresentStructuredContent(raw json.RawMessage, outputSchema schemaValidator) error {
	if len(raw) == 0 {
		return nil
	}
	_, value, err := canonicalJSON(raw)
	if err != nil {
		return &InvalidPayloadError{Subject: structuredContentSubject, Err: err}
	}
	if outputSchema != nil {
		if err := outputSchema.Validate(value); err != nil {
			return &InvalidPayloadError{Subject: structuredContentSubject, Err: err}
		}
	}
	return nil
}

func missingStructuredContentError() error {
	return &InvalidPayloadError{
		Subject: structuredContentSubject,
		Err:     errors.New("outputSchema requires structuredContent"),
	}
}

func (c *Client) nextProgressToken() string {
	return fmt.Sprintf("progress-%d-%d", time.Now().UnixNano(), c.progressCounter.Add(1))
}

func (c *Client) handleProgress(raw json.RawMessage) {
	var progress ProgressParams
	if err := json.Unmarshal(
		raw,
		&progress,
	); err != nil || progress.ProgressToken.IsZero() || math.IsNaN(progress.Progress) ||
		math.IsInf(progress.Progress, 0) || progress.Total != nil &&
		(math.IsNaN(*progress.Total) || math.IsInf(*progress.Total, 0)) {
		c.logger.Warn("mcp: invalid progress notification", "err", err)
		return
	}
	key, err := progressTokenKey(progress.ProgressToken)
	if err != nil {
		c.logger.Warn("mcp: invalid progress token", "err", err)
		return
	}
	value, ok := c.progressCallbacks.Load(key)
	if !ok {
		return
	}
	state, ok := value.(*progressState)
	if !ok {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.retired || state.started && progress.Progress <= state.last {
		return
	}
	state.started, state.last = true, progress.Progress
	chunk := toolsy.Chunk{
		Event: toolsy.EventProgress,
		Progress: &toolsy.ProgressInfo{
			Current: &progress.Progress,
			Total:   progress.Total,
			Message: progress.Message,
			Token:   progress.ProgressToken.String(),
		},
	}
	select {
	case state.ch <- chunk:
	case <-state.done:
	default:
		c.logger.Warn("mcp: progress notification dropped")
	}
}

func (c *Client) GetResourceTool() (toolsy.Tool, error) {
	if err := c.requireCapability("resources"); err != nil {
		return nil, err
	}
	schema := []byte(
		`{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"],"additionalProperties":false}`,
	)
	generation := c.resourceGeneration.Load()
	handler := func(ctx context.Context, _ *toolsy.RunEnv, argsJSON []byte, yield func(toolsy.Chunk) error) error {
		if current := c.resourceGeneration.Load(); current != generation {
			return staleError(InvalidationResources, generation, current)
		}
		var args struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return err
		}
		result, err := c.ReadResource(ctx, args.URI)
		if err != nil {
			return err
		}
		if current := c.resourceGeneration.Load(); current != generation {
			return staleError(InvalidationResources, generation, current)
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
			return fmt.Errorf("%w: %w", toolsy.ErrStreamAborted, err)
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

func (c *Client) ReadResource(ctx context.Context, uri string) (ResourcesReadResult, error) {
	result, input, err := c.ReadResourceRound(ctx, ResourcesReadParams{URI: uri})
	if input != nil {
		return ResourcesReadResult{}, &InputRequiredError{Method: MethodResourcesRead, Result: *input}
	}
	if err != nil {
		return ResourcesReadResult{}, err
	}
	return *result, nil
}

// ReadResourceRound executes one explicit resources/read MRTR round.
//

func (c *Client) ReadResourceRound(
	ctx context.Context,
	params ResourcesReadParams,
) (*ResourcesReadResult, *InputRequiredResult, error) {
	if err := validateOutboundInputResponses(MethodResourcesRead, params.InputResponses); err != nil {
		return nil, nil, err
	}
	if err := c.requireCapability("resources"); err != nil {
		return nil, nil, err
	}
	generation := c.resourceGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodResourcesRead, params)
	if err != nil {
		return nil, nil, c.mapCallReadLimitFor(ctx, err, "MCP resource read response")
	}
	if current := c.resourceGeneration.Load(); current != generation {
		return nil, nil, staleError(InvalidationResources, generation, current)
	}
	input, err := requireCompleteResult(raw, MethodResourcesRead, true)
	if err != nil || input != nil {
		return nil, input, err
	}
	var result ResourcesReadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, &InvalidPayloadError{Subject: "resources/read result", Err: err}
	}
	if _, err := formatResourceContents(result.Contents); err != nil {
		return nil, nil, &InvalidPayloadError{Subject: "resource contents", Err: err}
	}
	return &result, nil, nil
}

//nolint:nestif // A single resource has a lossless text/blob fast path with explicit MIME fallback.
func buildResourceResultChunk(result ResourcesReadResult, projection []byte) (toolsy.Chunk, error) {
	data, mimeType, delivery := projection, toolsy.MimeTypeText, toolsy.DeliveryClassText
	if len(result.Contents) == 1 {
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
		}
		if content.Blob != nil {
			decoded, err := decodeCanonicalBase64(*content.Blob)
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
	if len(data) == 0 {
		mimeType = ""
	}
	return toolsy.Chunk{
		Event:       toolsy.EventResult,
		Data:        data,
		MimeType:    mimeType,
		TypedResult: result,
		EmptyResult: len(result.Contents) == 0,
		Envelope:    toolsy.NewResultEnvelope(result, data, mimeType, delivery, toolsy.AudienceModel, nil),
	}, nil
}

func (c *Client) ListResources(ctx context.Context, cursor string) (ResourcesListResult, error) {
	if err := c.requireCapability("resources"); err != nil {
		return ResourcesListResult{}, err
	}
	generation := c.resourceGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodResourcesList, ResourcesListParams{Cursor: cursor})
	if err != nil {
		return ResourcesListResult{}, c.mapCallReadLimitFor(ctx, err, "MCP resources list response")
	}
	if current := c.resourceGeneration.Load(); current != generation {
		return ResourcesListResult{}, staleError(InvalidationResources, generation, current)
	}
	if _, err := requireCompleteResult(raw, MethodResourcesList, false); err != nil {
		return ResourcesListResult{}, err
	}
	var result ResourcesListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, &InvalidPayloadError{Subject: "resources/list result", Err: err}
	}
	return result, nil
}

func (c *Client) ListResourceTemplates(ctx context.Context, cursor string) (ResourceTemplatesListResult, error) {
	if err := c.requireCapability("resources"); err != nil {
		return ResourceTemplatesListResult{}, err
	}
	generation := c.resourceGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodResourceTemplatesList, ResourceTemplatesListParams{Cursor: cursor})
	if err != nil {
		return ResourceTemplatesListResult{}, c.mapCallReadLimitFor(ctx, err, "MCP resource templates list response")
	}
	if current := c.resourceGeneration.Load(); current != generation {
		return ResourceTemplatesListResult{}, staleError(InvalidationResources, generation, current)
	}
	if _, err := requireCompleteResult(raw, MethodResourceTemplatesList, false); err != nil {
		return ResourceTemplatesListResult{}, err
	}
	var result ResourceTemplatesListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, &InvalidPayloadError{Subject: "resources/templates/list result", Err: err}
	}
	return result, nil
}

func (c *Client) GetPrompts(ctx context.Context) iter.Seq2[Prompt, error] {
	if err := c.requireCapability("prompts"); err != nil {
		return errorSequence[Prompt](err)
	}
	generation := c.promptGeneration.Load()
	fetch := func(ctx context.Context, cursor string) ([]Prompt, string, error) {
		if current := c.promptGeneration.Load(); current != generation {
			return nil, "", staleError(InvalidationPrompts, generation, current)
		}
		result, err := c.ListPrompts(ctx, cursor)
		if err != nil {
			return nil, "", err
		}
		for _, prompt := range result.Prompts {
			if err := validatePrompt(prompt); err != nil {
				return nil, "", &InvalidPayloadError{Subject: "prompt descriptor", Err: err}
			}
		}
		if current := c.promptGeneration.Load(); current != generation {
			return nil, "", staleError(InvalidationPrompts, generation, current)
		}
		return result.Prompts, result.NextCursor, nil
	}
	return IterateCursorWithLimits(ctx, c.opts.Pagination, fetch)
}

func (c *Client) ListPrompts(ctx context.Context, cursor string) (PromptsListResult, error) {
	if err := c.requireCapability("prompts"); err != nil {
		return PromptsListResult{}, err
	}
	generation := c.promptGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodPromptsList, PromptsListParams{Cursor: cursor})
	if err != nil {
		return PromptsListResult{}, c.mapCallReadLimitFor(ctx, err, "MCP prompts list response")
	}
	if current := c.promptGeneration.Load(); current != generation {
		return PromptsListResult{}, staleError(InvalidationPrompts, generation, current)
	}
	if _, err := requireCompleteResult(raw, MethodPromptsList, false); err != nil {
		return PromptsListResult{}, err
	}
	var result PromptsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, &InvalidPayloadError{Subject: "prompts/list result", Err: err}
	}
	return result, nil
}

func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*PromptsGetResult, error) {
	result, input, err := c.GetPromptRound(ctx, PromptsGetParams{Name: name, Arguments: args})
	if input != nil {
		return nil, &InputRequiredError{Method: MethodPromptsGet, Result: *input}
	}
	return result, err
}

// GetPromptRound executes one explicit prompts/get MRTR round.
func (c *Client) GetPromptRound(
	ctx context.Context,
	params PromptsGetParams,
) (*PromptsGetResult, *InputRequiredResult, error) {
	if err := validateOutboundInputResponses(MethodPromptsGet, params.InputResponses); err != nil {
		return nil, nil, err
	}
	if err := c.requireCapability("prompts"); err != nil {
		return nil, nil, err
	}
	generation := c.promptGeneration.Load()
	raw, err := c.requestAndAwait(ctx, MethodPromptsGet, params)
	if err != nil {
		return nil, nil, c.mapCallReadLimitFor(ctx, err, "MCP prompt response")
	}
	if current := c.promptGeneration.Load(); current != generation {
		return nil, nil, staleError(InvalidationPrompts, generation, current)
	}
	input, err := requireCompleteResult(raw, MethodPromptsGet, true)
	if err != nil || input != nil {
		return nil, input, err
	}
	var result PromptsGetResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, &InvalidPayloadError{Subject: "prompts/get result", Err: err}
	}
	for _, message := range result.Messages {
		if _, err := formatContentBlocks([]ContentBlock{message.Content}); err != nil {
			return nil, nil, &InvalidPayloadError{Subject: "prompt content", Err: err}
		}
	}
	return &result, nil, nil
}

func validateOutboundInputResponses(method string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if err := validateInputResponses(raw); err != nil {
		return &InvalidPayloadError{Subject: method + " inputResponses", Err: err}
	}
	return nil
}

func subscriptionIdentity(meta Meta) (string, string, bool) {
	raw, ok := meta[metaSubscriptionID]
	if !ok {
		return "", "", false
	}
	key, err := rpcIDKey(raw)
	if err != nil {
		return "", "", false
	}
	var id any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&id) != nil {
		return "", "", false
	}
	switch value := id.(type) {
	case string:
		return key, value, true
	case json.Number:
		return key, value.String(), true
	default:
		return "", "", false
	}
}

func (c *Client) Listen(ctx context.Context, filter SubscriptionFilter) (*Subscription, error) {
	filter = cloneSubscriptionFilter(filter)
	if !filter.ToolsListChanged && !filter.ResourcesListChanged && !filter.PromptsListChanged &&
		len(filter.ResourceSubscriptions) == 0 {
		return nil, &InvalidPayloadError{
			Subject: MethodSubscriptionsListen,
			Err:     errors.New("at least one notification must be selected"),
		}
	}
	if err := c.validateSubscriptionFilter(filter); err != nil {
		return nil, err
	}
	listenCtx, cancel := context.WithCancel(ctx)
	pending, logKey, err := c.prepareRequest(
		listenCtx,
		MethodSubscriptionsListen,
		SubscriptionsListenParams{Notifications: filter},
	)
	if err != nil {
		cancel()
		return nil, err
	}
	key, err := rpcIDKey(pending.ID())
	if err != nil {
		cancel()
		_ = pending.Abort(err)
		return nil, &InvalidPayloadError{Subject: "subscriptions/listen request id", Err: err}
	}
	_, publicID, _ := subscriptionIdentity(Meta{metaSubscriptionID: pending.ID()})
	state := &subscriptionState{
		key:       key,
		publicID:  publicID,
		requested: cloneSubscriptionFilter(filter),
		events:    make(chan Invalidation, clientEventBufferSize),
		done:      make(chan struct{}),
		cancel:    cancel,
		ctx:       listenCtx,
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel()
		_ = pending.Abort(ErrTransportClosed)
		return nil, ErrTransportClosed
	}
	c.pruneRetiredSubscriptions(time.Now())
	limits := c.opts.Subscriptions
	if limits.MaxTracked == 0 {
		limits, _ = limits.normalized()
	}
	if len(c.subscriptions) >= limits.MaxActive ||
		len(c.subscriptions)+len(c.retiredSubscriptions) >= limits.MaxTracked {
		c.mu.Unlock()
		cancel()
		limitErr := &InvalidPayloadError{
			Subject: MethodSubscriptionsListen,
			Err:     errors.New("subscription correlation capacity exceeded"),
		}
		_ = pending.Abort(limitErr)
		return nil, limitErr
	}
	_, retired := c.retiredSubscriptions[key]
	if _, duplicate := c.subscriptions[key]; duplicate || retired {
		c.mu.Unlock()
		cancel()
		duplicateErr := &InvalidPayloadError{
			Subject: "subscriptions/listen request id",
			Err:     errors.New("duplicate active or retired request id"),
		}
		_ = pending.Abort(duplicateErr)
		return nil, duplicateErr
	}
	c.subscriptions[key] = state
	c.mu.Unlock()
	if err := c.deliverPrepared(pending, logKey); err != nil {
		c.mu.Lock()
		delete(c.subscriptions, key)
		c.mu.Unlock()
		cancel()
		return nil, err
	}
	go c.awaitSubscription(listenCtx, pending, state)
	return &Subscription{
		ID:     publicID,
		Events: state.events,
		done:   state.done,
		cancel: func() { cancelSubscriptionLocally(state) },
		err:    func() error { state.mu.RLock(); defer state.mu.RUnlock(); return state.err },
	}, nil
}

func (c *Client) awaitSubscription(ctx context.Context, pending PendingRequest, state *subscriptionState) {
	raw, err := c.awaitPending(ctx, pending)
	state.mu.RLock()
	violationErr := state.err
	state.mu.RUnlock()
	if violationErr != nil {
		err = violationErr
	}
	if err == nil {
		_, err = requireCompleteResult(raw, MethodSubscriptionsListen, false)
	}
	if err == nil {
		var result SubscriptionsListenResult
		if decodeErr := json.Unmarshal(raw, &result); decodeErr != nil {
			err = decodeErr
		} else {
			terminalKey, _, valid := subscriptionIdentity(Meta{metaSubscriptionID: result.Meta.SubscriptionID})
			if !valid || terminalKey != state.key {
				err = errors.New("subscription terminal result ID mismatch")
			}
		}
	}
	state.mu.Lock()
	if err == nil && !state.acked {
		err = errors.New("subscription ended before acknowledgment")
	}
	retire := locallyCanceledSubscription(state)
	state.err = err
	state.closed = true
	state.mu.Unlock()
	c.mu.Lock()
	delete(c.subscriptions, state.key)
	if retire && !c.closed {
		if c.retiredSubscriptions == nil {
			c.retiredSubscriptions = make(map[string]time.Time)
		}
		limits := c.opts.Subscriptions
		if limits.RetireTTL == 0 {
			limits, _ = limits.normalized()
		}
		c.retiredSubscriptions[state.key] = time.Now().Add(limits.RetireTTL)
	}
	c.mu.Unlock()
	state.mu.Lock()
	close(state.events)
	close(state.done)
	state.mu.Unlock()
}

func failSubscription(state *subscriptionState, err error) {
	state.mu.Lock()
	if state.err == nil {
		state.err = err
	}
	state.mu.Unlock()
	state.cancel()
}

func (c *Client) failAllSubscriptions(err error) {
	c.mu.RLock()
	states := make([]*subscriptionState, 0, len(c.subscriptions))
	for _, state := range c.subscriptions {
		states = append(states, state)
	}
	c.mu.RUnlock()
	for _, state := range states {
		failSubscription(state, err)
	}
}

func (c *Client) validateSubscriptionFilter(filter SubscriptionFilter) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.ready {
		return &CapabilityError{Capability: "server/discover"}
	}
	if filter.ToolsListChanged && (c.server.Capabilities.Tools == nil || !c.server.Capabilities.Tools.ListChanged) {
		return &CapabilityError{Capability: "tools.listChanged"}
	}
	if filter.ResourcesListChanged &&
		(c.server.Capabilities.Resources == nil || !c.server.Capabilities.Resources.ListChanged) {
		return &CapabilityError{Capability: "resources.listChanged"}
	}
	if filter.PromptsListChanged &&
		(c.server.Capabilities.Prompts == nil || !c.server.Capabilities.Prompts.ListChanged) {
		return &CapabilityError{Capability: "prompts.listChanged"}
	}
	if len(filter.ResourceSubscriptions) > 0 &&
		(c.server.Capabilities.Resources == nil || !c.server.Capabilities.Resources.Subscribe) {
		return &CapabilityError{Capability: "resources.subscribe"}
	}
	return nil
}

func (c *Client) handleSubscriptionAcknowledged(raw json.RawMessage) {
	var ack SubscriptionsAcknowledgedParams
	if err := json.Unmarshal(raw, &ack); err != nil {
		c.logger.Warn("mcp: invalid subscription acknowledgment", "err", err)
		c.failAllSubscriptions(errors.New("invalid subscription acknowledgment"))
		return
	}
	key, _, ok := subscriptionIdentity(ack.Meta)
	if !ok {
		c.logger.Warn("mcp: subscription acknowledgment missing ID")
		c.failAllSubscriptions(errors.New("subscription acknowledgment missing or invalid ID"))
		return
	}
	state, retired := c.subscriptionRoute(key)
	if retired {
		return
	}
	if state == nil {
		c.logger.Warn("mcp: unknown subscription acknowledgment")
		c.failAllSubscriptions(errors.New("subscription acknowledgment has unknown ID"))
		return
	}
	state.mu.RLock()
	canceled := locallyCanceledSubscription(state)
	state.mu.RUnlock()
	if canceled {
		return
	}
	if err := validateEffectiveSubscriptionFilter(state.requested, ack.Notifications); err != nil {
		failSubscription(state, err)
		return
	}
	state.mu.Lock()
	if locallyCanceledSubscription(state) {
		state.mu.Unlock()
		return
	}
	if state.closed {
		state.mu.Unlock()
		failSubscription(state, errors.New("subscription acknowledgment after close"))
		return
	}
	if state.acked {
		state.mu.Unlock()
		failSubscription(state, errors.New("duplicate subscription acknowledgment"))
		return
	}
	state.effective, state.acked = ack.Notifications, true
	state.mu.Unlock()
}

func validateEffectiveSubscriptionFilter(requested, effective SubscriptionFilter) error {
	if effective.ToolsListChanged && !requested.ToolsListChanged ||
		effective.ResourcesListChanged && !requested.ResourcesListChanged ||
		effective.PromptsListChanged && !requested.PromptsListChanged {
		return errors.New("subscription acknowledgment expands the requested filter")
	}
	seen := make(map[string]struct{}, len(effective.ResourceSubscriptions))
	for _, uri := range effective.ResourceSubscriptions {
		if !slices.Contains(requested.ResourceSubscriptions, uri) {
			return errors.New("subscription acknowledgment includes an unrequested resource URI")
		}
		if _, duplicate := seen[uri]; duplicate {
			return errors.New("subscription acknowledgment contains a duplicate resource URI")
		}
		seen[uri] = struct{}{}
	}
	return nil
}

//nolint:funlen // Fail-closed subscription validation precedes every generation mutation.
func (c *Client) handleSubscriptionInvalidation(kind InvalidationKind, raw json.RawMessage) {
	fields, err := decodeObjectFields(raw)
	if err != nil {
		c.logger.Warn("mcp: invalid subscription notification", "err", err)
		c.failAllSubscriptions(errors.New("invalid subscription notification"))
		return
	}
	var meta Meta
	if err := json.Unmarshal(fields["_meta"], &meta); err != nil {
		c.logger.Warn("mcp: subscription notification missing metadata")
		c.failAllSubscriptions(errors.New("subscription notification missing metadata"))
		return
	}
	key, publicID, ok := subscriptionIdentity(meta)
	if !ok {
		c.logger.Warn("mcp: subscription notification missing ID")
		c.failAllSubscriptions(errors.New("subscription notification missing or invalid ID"))
		return
	}
	state, retired := c.subscriptionRoute(key)
	if retired {
		c.validateRetiredSubscriptionNotification(kind, raw)
		return
	}
	if state == nil {
		c.logger.Warn("mcp: notification for unknown subscription")
		c.failAllSubscriptions(errors.New("subscription notification has unknown ID"))
		return
	}
	state.mu.RLock()
	if locallyCanceledSubscription(state) {
		state.mu.RUnlock()
		c.validateRetiredSubscriptionNotification(kind, raw)
		return
	}
	closed := state.closed
	acked := state.acked
	effective := cloneSubscriptionFilter(state.effective)
	state.mu.RUnlock()
	allowed := !closed && acked && subscriptionAllows(effective, kind)
	if !allowed {
		failSubscription(
			state,
			errors.New("subscription notification before acknowledgment or outside effective filter"),
		)
		return
	}
	uri := ""
	if kind == InvalidationResource {
		var updated ResourceUpdatedParams
		if json.Unmarshal(raw, &updated) != nil || updated.URI == "" {
			failSubscription(state, errors.New("invalid resource subscription notification"))
			return
		}
		uri = updated.URI
		allowed = resourceSubscriptionAllows(effective.ResourceSubscriptions, uri)
		if !allowed {
			failSubscription(state, errors.New("resource notification outside effective subscription filter"))
			return
		}
	}
	event := c.advanceInvalidation(kind, uri, publicID)
	state.mu.RLock()
	if state.closed {
		state.mu.RUnlock()
		return
	}
	select {
	case state.events <- event:
	default:
		c.logger.Warn("mcp: subscription event channel full", "subscription", boundedDiagnostic(publicID))
	}
	state.mu.RUnlock()
}

func cloneSubscriptionFilter(filter SubscriptionFilter) SubscriptionFilter {
	filter.ResourceSubscriptions = append([]string(nil), filter.ResourceSubscriptions...)
	filter.Extra = cloneMeta(filter.Extra)
	return filter
}

func resourceSubscriptionAllows(subscriptions []string, uri string) bool {
	for _, subscription := range subscriptions {
		if subscription == uri || subscriptionURIMatches(subscription, uri, true) {
			return true
		}
	}
	return false
}

func subscriptionURIMatches(subscription, candidate string, allowEqual bool) bool {
	base, baseErr := url.Parse(subscription)
	updated, updatedErr := url.Parse(candidate)
	if baseErr != nil || updatedErr != nil || base.IsAbs() != updated.IsAbs() ||
		!strings.EqualFold(base.Scheme, updated.Scheme) || !strings.EqualFold(base.Hostname(), updated.Hostname()) ||
		subscriptionURIPort(base) != subscriptionURIPort(updated) ||
		base.User.String() != updated.User.String() || base.Opaque != "" || updated.Opaque != "" {
		return false
	}
	if hasDotPathSegment(base.Path) || hasDotPathSegment(updated.Path) {
		return false
	}
	basePath := normalizeSubscriptionPath(base.EscapedPath())
	updatedPath := normalizeSubscriptionPath(updated.EscapedPath())
	baseHasFragment := strings.Contains(subscription, "#")
	updatedHasFragment := strings.Contains(candidate, "#")
	if allowEqual && basePath == updatedPath && base.RawQuery == updated.RawQuery &&
		base.ForceQuery == updated.ForceQuery && baseHasFragment == updatedHasFragment &&
		base.EscapedFragment() == updated.EscapedFragment() {
		return true
	}
	if base.RawQuery != "" || updated.RawQuery != "" || base.ForceQuery || updated.ForceQuery ||
		baseHasFragment || updatedHasFragment {
		return false
	}
	if basePath == "" || updatedPath == basePath {
		return false
	}
	if strings.HasSuffix(basePath, "/") {
		return strings.HasPrefix(updatedPath, basePath)
	}
	return strings.HasPrefix(updatedPath, basePath+"/")
}

func subscriptionURIPort(uri *url.URL) string {
	port := uri.Port()
	if strings.EqualFold(uri.Scheme, "http") && port == "80" ||
		strings.EqualFold(uri.Scheme, "https") && port == "443" {
		return ""
	}
	return port
}

// Normalize only unreserved octets. In particular, encoded slashes must never
// become path separators, and repeated separators are never collapsed.
func normalizeSubscriptionPath(path string) string {
	var normalized strings.Builder
	const hex = "0123456789ABCDEF"
	for index := 0; index < len(path); index++ {
		if path[index] != '%' || index+2 >= len(path) {
			normalized.WriteByte(path[index])
			continue
		}
		decoded, err := url.PathUnescape(path[index : index+3])
		if err != nil {
			normalized.WriteByte(path[index])
			continue
		}
		octet := decoded[0]
		if octet >= 'a' && octet <= 'z' || octet >= 'A' && octet <= 'Z' ||
			octet >= '0' && octet <= '9' || strings.ContainsRune("-._~", rune(octet)) {
			normalized.WriteByte(octet)
		} else {
			normalized.WriteByte('%')
			normalized.WriteByte(hex[octet>>4])
			normalized.WriteByte(hex[octet&15])
		}
		index += 2
	}
	return normalized.String()
}

func hasDotPathSegment(path string) bool {
	for segment := range strings.SplitSeq(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func subscriptionAllows(filter SubscriptionFilter, kind InvalidationKind) bool {
	switch kind {
	case InvalidationTools:
		return filter.ToolsListChanged
	case InvalidationResources:
		return filter.ResourcesListChanged
	case InvalidationPrompts:
		return filter.PromptsListChanged
	case InvalidationResource:
		return len(filter.ResourceSubscriptions) > 0
	default:
		return false
	}
}

func (c *Client) advanceInvalidation(kind InvalidationKind, uri, subscription string) Invalidation {
	generation := c.invalidationCount.Add(1)
	switch kind {
	case InvalidationTools:
		c.toolGeneration.Add(1)
		c.clearToolBindings()
	case InvalidationResources, InvalidationResource:
		c.resourceGeneration.Add(1)
	case InvalidationPrompts:
		c.promptGeneration.Add(1)
	}
	event := Invalidation{Kind: kind, URI: uri, Generation: generation, SubscriptionID: subscription}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return event
	}
	select {
	case c.invalidations <- event:
	default:
		c.logger.Warn("mcp: invalidation event channel full")
	}
	return event
}

func (c *Client) clearToolBindings() {
	c.toolBindingMu.Lock()
	defer c.toolBindingMu.Unlock()
	if headerTransport, ok := c.transport.(ToolHeaderTransport); ok {
		_ = headerTransport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{})
	}
	clear(c.toolBindings)
	clear(c.toolSchemas)
}

func (c *Client) notifyCancelledPending(parent context.Context, pending PendingRequest, reason string) {
	if notification, ok := pending.(CancellationNotificationPending); ok &&
		!notification.ClaimCancellationNotification() {
		return
	}
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
	reason = strings.Map(func(char rune) rune {
		if char == '\n' || char == '\r' || char == '\t' {
			return ' '
		}
		return char
	}, reason)
	runes := []rune(reason)
	if len(runes) > maxCancellationRunes {
		reason = string(runes[:maxCancellationRunes])
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), cancelNotifyTimeout)
	defer cancel()
	_ = c.transport.Notify(ctx, MethodCancelled, CancelledParams{RequestID: pending.ID(), Reason: reason})
}

func (c *Client) Invalidations() <-chan Invalidation { return c.invalidations }
func (c *Client) InvalidationGeneration() uint64     { return c.invalidationCount.Load() }
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

func (c *Client) ServerInfo() DiscoverResult {
	c.mu.RLock()
	raw, err := json.Marshal(c.server)
	c.mu.RUnlock()
	if err != nil {
		return DiscoverResult{}
	}
	var result DiscoverResult
	if json.Unmarshal(raw, &result) != nil {
		return DiscoverResult{}
	}
	return result
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		subscriptions := make([]*subscriptionState, 0, len(c.subscriptions))
		for _, state := range c.subscriptions {
			subscriptions = append(subscriptions, state)
		}
		c.mu.Unlock()
		for _, state := range subscriptions {
			state.cancel()
		}
		c.closeErr = c.transport.Close()
		c.mu.Lock()
		close(c.invalidations)
		c.mu.Unlock()
	})
	return c.closeErr
}
