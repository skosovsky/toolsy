//nolint:exhaustruct_v5 // Internal transport values intentionally omit optional fields.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"mime"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	mcpProtocolVersionHeader          = "MCP-Protocol-Version"
	mcpMethodHeader                   = "Mcp-Method"
	mcpNameHeader                     = "Mcp-Name"
	mcpParamHeaderPrefix              = "Mcp-Param-"
	eventStreamMediaType              = "text/event-stream"
	streamableHTTPResponseSubject     = "Streamable HTTP response"
	streamableHTTPSSESubject          = "Streamable HTTP SSE"
	streamableHTTPJSONResponseSubject = "Streamable HTTP JSON response"
	streamableHTTPRequestSubject      = "Streamable HTTP request"
	httpDecoratorSubject              = "HTTP request decorator"
	maxSafeInteger                    = int64(1<<53 - 1)
)

type HTTPRequestDecorator func(*http.Request) error
type StreamableHTTPOption func(*StreamableHTTPTransport)

func WithStreamableHTTPLogger(logger *slog.Logger) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) {
		if logger != nil {
			transport.logger = logger
		}
	}
}

func WithStreamableHTTPAllowPrivateIPs(allow bool) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) { transport.allowPrivateIPs = allow }
}

// WithStreamableHTTPSettings configures timeout/TLS on the owned safe pool.
// Invalid settings fail Start before any remote dispatch.
func WithStreamableHTTPSettings(settings httptool.ClientSettings) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) { transport.httpSettings = settings }
}

func WithStreamableHTTPRequestDecorator(decorator HTTPRequestDecorator) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) { transport.decorator = decorator }
}

type StreamableHTTPTransport struct {
	endpoint     string
	logger       *slog.Logger
	client       *http.Client
	httpSettings httptool.ClientSettings
	settingsErr  error
	decorator    HTTPRequestDecorator

	allowPrivateIPs bool
	maxFrameBytes   int
	limits          TransportLimits
	limitsErr       error
	queuedBytes     int
	mu              sync.RWMutex
	started         bool
	closed          bool
	terminating     bool
	terminalErr     error
	lifetimeCtx     context.Context
	cancel          context.CancelFunc
	peer            *rpcPeer
	notifyHandlers  map[string]NotificationHandler
	requestHandlers map[string]RequestScopedNotificationHandler
	toolHeaders     map[string][]HTTPToolHeaderBinding
	activePosts     map[uint64]context.CancelFunc
	nextPostID      uint64
	postWG          sync.WaitGroup
	closeOnce       sync.Once
}

func NewStreamableHTTPTransport(endpoint string, opts ...StreamableHTTPOption) *StreamableHTTPTransport {
	transport := &StreamableHTTPTransport{
		endpoint: endpoint,
		logger:   slog.Default(),

		notifyHandlers:  make(map[string]NotificationHandler),
		requestHandlers: make(map[string]RequestScopedNotificationHandler),
		toolHeaders:     make(map[string][]HTTPToolHeaderBinding),
		activePosts:     make(map[uint64]context.CancelFunc),
	}
	for _, opt := range opts {
		if opt == nil {
			transport.limitsErr = errors.New("mcp: nil HTTP option")
			continue
		}
		opt(transport)
	}
	normalized, err := transport.limits.normalized()
	if err != nil {
		normalized, _ = (TransportLimits{}).normalized()
	}
	if transport.limitsErr == nil {
		transport.limitsErr = err
	}
	transport.limits = normalized
	transport.maxFrameBytes = normalized.MaxFrameBytes
	transport.client, transport.settingsErr = httptool.NewConfiguredSafeHTTPClient(
		httptool.SafeDialOptions{AllowPrivateIPs: transport.allowPrivateIPs},
		httptool.CheckRedirectRemote(transport.allowPrivateIPs, nil), transport.httpSettings,
	)
	return transport
}

func (t *StreamableHTTPTransport) Start(ctx context.Context) error {
	if t.limitsErr != nil {
		return t.limitsErr
	}
	if t.settingsErr != nil {
		return t.settingsErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransportClosed
	}
	if t.started {
		return nil
	}
	if err := httptool.ValidateRemoteURL(ctx, t.endpoint, t.allowPrivateIPs); err != nil {
		return err
	}

	t.lifetimeCtx, t.cancel = context.WithCancel(context.WithoutCancel(ctx))
	t.peer = newRPCPeer(t.lifetimeCtx, t.logger, func(context.Context, []byte) error {
		return errors.New("mcp: HTTP transport cannot send server responses")
	})
	t.peer.limits = t.limits
	for method, handler := range t.notifyHandlers {
		t.peer.setNotificationHandler(method, handler)
	}
	t.started = true
	return nil
}

func (t *StreamableHTTPTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateOutgoingRequest(method, params); err != nil {
		return nil, err
	}
	t.mu.RLock()
	peer := t.peer
	started := t.started && !t.closed && !t.terminating
	t.mu.RUnlock()
	if !started || peer == nil {
		return nil, ErrTransportClosed
	}
	pending, body, err := peer.beginRequest(method, params)
	if err != nil {
		return nil, err
	}
	routingHeaders, err := t.routingHeaders(body)
	if err != nil {
		peer.failPending(pending, err)
		return nil, err
	}
	return &preparedRequest{
		pendingRequest: pending,
		ctx:            ctx,
		deliver: func() {
			go func() {
				postCtx, stopPost := contextUntilPendingTerminal(ctx, pending.terminal)
				defer stopPost()
				defer pending.finishDelivery()
				if postErr := t.postMessageTracked(
					postCtx,
					body,
					routingHeaders,
					pending.markSent,
				); postErr != nil {
					peer.failPending(pending, t.failureCause(postErr))
				}
			}()
		},
	}, nil
}

// Notify is intentionally unavailable on Streamable HTTP in MCP 2026-07-28.
// HTTP cancellation is represented solely by closing the request response body.
func (t *StreamableHTTPTransport) Notify(_ context.Context, method string, params any) error {
	if err := validateOutgoingNotification(method, params); err != nil {
		return err
	}
	return &InvalidPayloadError{
		Subject: "Streamable HTTP notification",
		Err:     errors.New("client notifications are not defined for MCP 2026-07-28 Streamable HTTP"),
	}
}

func (t *StreamableHTTPTransport) OnNotification(method string, handler NotificationHandler) {
	t.mu.Lock()
	if handler == nil {
		delete(t.notifyHandlers, method)
	} else {
		t.notifyHandlers[method] = handler
	}
	peer := t.peer
	t.mu.Unlock()
	if peer != nil {
		peer.setNotificationHandler(method, handler)
	}
}

func (t *StreamableHTTPTransport) OnRequestNotification(
	method string,
	handler RequestScopedNotificationHandler,
) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if handler == nil {
		delete(t.requestHandlers, method)
		return
	}
	t.requestHandlers[method] = handler
}

func validateHTTPToolHeaderBindings(bindings []HTTPToolHeaderBinding) ([]HTTPToolHeaderBinding, error) {
	validated := make([]HTTPToolHeaderBinding, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for index, binding := range bindings {
		if !validHTTPToken(binding.Header) {
			return nil, fmt.Errorf("mcp: invalid x-mcp-header %q", binding.Header)
		}
		key := strings.ToLower(binding.Header)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("mcp: duplicate x-mcp-header %q", binding.Header)
		}
		seen[key] = struct{}{}
		if len(binding.Path) != 1 || binding.Path[0] == "" {
			return nil, fmt.Errorf("mcp: x-mcp-header %q must use one top-level property", binding.Header)
		}
		switch binding.Type {
		case schemaTypeString, schemaTypeInteger, schemaTypeBoolean:
		default:
			return nil, fmt.Errorf("mcp: x-mcp-header %q has unsupported type %q", binding.Header, binding.Type)
		}
		validated[index] = HTTPToolHeaderBinding{
			Header: binding.Header,
			Path:   append([]string(nil), binding.Path...),
			Type:   binding.Type,
		}
	}
	return validated, nil
}

func (t *StreamableHTTPTransport) ReplaceToolHeaderBindings(
	bindings map[string][]HTTPToolHeaderBinding,
) error {
	validated := make(map[string][]HTTPToolHeaderBinding, len(bindings))
	for tool, descriptors := range bindings {
		if tool == "" {
			return errors.New("mcp: tool header binding requires a tool name")
		}
		toolBindings, err := validateHTTPToolHeaderBindings(descriptors)
		if err != nil {
			return fmt.Errorf("mcp: tool %q header bindings: %w", tool, err)
		}
		validated[tool] = toolBindings
	}
	t.mu.Lock()
	t.toolHeaders = validated
	t.mu.Unlock()
	return nil
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		char := value[index]
		if (char < '0' || char > '9') && (char < 'A' || char > 'Z') &&
			(char < 'a' || char > 'z') && !strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			return false
		}
	}
	return true
}

func (t *StreamableHTTPTransport) postMessageTracked(
	ctx context.Context,
	body []byte,
	routingHeaders map[string]string,
	onSent func(),
) error {
	postCtx, cancelPost := context.WithCancel(ctx)
	t.mu.Lock()
	if t.closed || t.terminating {
		terminalErr := t.terminalErr
		t.mu.Unlock()
		cancelPost()
		if terminalErr != nil {
			return terminalErr
		}
		return ErrTransportClosed
	}
	if err := retainedWorkLimit(len(t.activePosts), t.queuedBytes, len(body), t.limits); err != nil {
		t.mu.Unlock()
		cancelPost()
		return &InvalidPayloadError{Subject: "HTTP outgoing queue", Err: err}
	}
	t.queuedBytes += len(body)
	t.nextPostID++
	postID := t.nextPostID
	t.activePosts[postID] = cancelPost
	t.postWG.Add(1)
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.activePosts, postID)
		t.queuedBytes -= len(body)
		t.mu.Unlock()
		cancelPost()
		t.postWG.Done()
	}()
	err := t.postMessageOnce(postCtx, body, routingHeaders, onSent)
	if isTerminalHTTPTransportError(err) {
		go t.terminate(err)
	}
	return err
}

func (t *StreamableHTTPTransport) postMessageOnce(
	ctx context.Context,
	body []byte,
	routingHeaders map[string]string,
	onSent func(),
) error {
	if onSent != nil {
		var sentOnce sync.Once
		trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				sentOnce.Do(onSent)
			}
		}}
		ctx = httptrace.WithClientTrace(ctx, trace)
	}
	request, err := t.buildRequest(ctx, body, routingHeaders)
	if err != nil {
		return err
	}
	provenance, err := newSSERequestProvenance(body)
	if err != nil {
		return err
	}
	// #nosec G704 -- endpoint validation and safe dialing are enforced in Start/default client.
	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		// Authentication status is sufficient; do not wait for an untrusted body.
		_ = response.Body.Close()
		return authenticationHTTPError(response)
	}
	defer httptool.CloseResponseBody(ctx, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return t.consumeHTTPError(ctx, response, body)
	}
	if response.StatusCode == http.StatusAccepted {
		return &InvalidPayloadError{
			Subject: streamableHTTPResponseSubject,
			Err:     errors.New("202 Accepted is invalid for a JSON-RPC request"),
		}
	}
	return t.consumeSuccessfulHTTPResponse(ctx, response, body, provenance)
}

func (t *StreamableHTTPTransport) consumeSuccessfulHTTPResponse(
	ctx context.Context,
	response *http.Response,
	requestBody []byte,
	provenance *sseRequestProvenance,
) error {
	contentType, err := exactMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return &InvalidPayloadError{Subject: "Streamable HTTP Content-Type", Err: err}
	}
	switch contentType {
	case "application/json":
		payload, readErr := httptool.ReadBodyLimited(ctx, response.Body, t.maxFrameBytes)
		if readErr != nil {
			return readErr
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: errors.New("empty body")}
		}
		if err := validateJSONPostCorrelation(requestBody, payload); err != nil {
			return err
		}
		if err := rejectReservedRPCErrorOutsideHTTP400(payload); err != nil {
			return err
		}
		return t.peer.dispatch(payload)
	case eventStreamMediaType:
		return t.consumeRequestSSE(ctx, response.Body, provenance)
	default:
		return &InvalidPayloadError{
			Subject: streamableHTTPResponseSubject,
			Err:     fmt.Errorf("unsupported content type %q", contentType),
		}
	}
}

func (t *StreamableHTTPTransport) consumeHTTPError(
	ctx context.Context,
	response *http.Response,
	requestBody []byte,
) error {
	payload, err := httptool.ReadBodyLimited(ctx, response.Body, t.maxFrameBytes)
	if err == nil && len(bytes.TrimSpace(payload)) > 0 &&
		validateNon2xxRPCError(response.StatusCode, requestBody, payload) == nil &&
		t.peer.dispatch(payload) == nil {
		return nil
	}
	return &HTTPError{StatusCode: response.StatusCode, Operation: "POST"}
}

func (t *StreamableHTTPTransport) buildRequest(
	ctx context.Context,
	body []byte,
	routingHeaders map[string]string,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, nil)
	if err != nil {
		return nil, err
	}
	t.mu.RLock()
	decorator := t.decorator
	t.mu.RUnlock()
	if decorator != nil {
		if decoratorErr := applyRequestDecorator(request, decorator); decoratorErr != nil {
			return nil, decoratorErr
		}
	}
	for name := range request.Header {
		if isReservedMCPHeader(name) {
			return nil, &InvalidPayloadError{
				Subject: httpDecoratorSubject,
				Err:     fmt.Errorf("decorator must not set reserved MCP header %q", name),
			}
		}
	}
	decoratedHeaders := request.Header.Clone()
	request, err = http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header = decoratedHeaders
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	for name, value := range routingHeaders {
		request.Header.Set(name, value)
	}
	return request, nil
}

func isReservedMCPHeader(name string) bool {
	lowerName := strings.ToLower(name)
	return strings.HasPrefix(lowerName, "mcp-") || strings.HasPrefix(lowerName, "last-event-")
}

func applyRequestDecorator(request *http.Request, decorator HTTPRequestDecorator) error {
	originalURL := request.URL.String()
	originalMethod := request.Method
	originalHost := request.Host
	if err := decorator(request); err != nil {
		return err
	}
	if request.URL.String() != originalURL || request.Method != originalMethod || request.Host != originalHost {
		return &InvalidPayloadError{
			Subject: httpDecoratorSubject,
			Err:     errors.New("decorator must not change request method, URL, or Host override"),
		}
	}
	if !requestDecoratorChangedBody(request) {
		return nil
	}
	if request.Body != nil {
		_ = request.Body.Close()
	}
	return &InvalidPayloadError{
		Subject: httpDecoratorSubject,
		Err:     errors.New("decorator must not set request body fields"),
	}
}

func requestDecoratorChangedBody(request *http.Request) bool {
	return request.Body != nil || request.GetBody != nil || request.ContentLength != 0 ||
		len(request.TransferEncoding) > 0 || len(request.Trailer) > 0 ||
		request.Header.Get("Content-Length") != "" || request.Header.Get("Transfer-Encoding") != "" ||
		request.Header.Get("Trailer") != ""
}

func (t *StreamableHTTPTransport) routingHeaders(body []byte) (map[string]string, error) {
	fields, err := decodeObjectFields(body)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: streamableHTTPRequestSubject, Err: err}
	}
	var method string
	if unmarshalErr := json.Unmarshal(fields["method"], &method); unmarshalErr != nil || method == "" {
		return nil, &InvalidPayloadError{Subject: mcpMethodHeader, Err: errors.New("body method is required")}
	}
	params, err := decodeObjectFields(fields["params"])
	if err != nil {
		return nil, &InvalidPayloadError{Subject: "Streamable HTTP params", Err: err}
	}
	if err := validateProtocolMeta(params["_meta"]); err != nil {
		return nil, err
	}
	headers := map[string]string{mcpProtocolVersionHeader: ProtocolVersion, mcpMethodHeader: method}
	nameField := ""
	switch method {
	case "tools/call", "prompts/get":
		nameField = "name"
	case "resources/read":
		nameField = "uri"
	}
	var name string
	if nameField != "" {
		if err := json.Unmarshal(params[nameField], &name); err != nil || name == "" {
			return nil, &InvalidPayloadError{
				Subject: mcpNameHeader,
				Err:     fmt.Errorf("params.%s must be a non-empty string", nameField),
			}
		}
		headers[mcpNameHeader] = encodeMCPHeaderValue(name)
	}
	if method == "tools/call" {
		t.mu.RLock()
		bindings, bindingsKnown := t.toolHeaders[name]
		bindings = append([]HTTPToolHeaderBinding(nil), bindings...)
		t.mu.RUnlock()
		if !bindingsKnown {
			return nil, &InvalidPayloadError{
				Subject: "tools/call HTTP header authority",
				Err:     fmt.Errorf("tool %q has no current tools/list header descriptor", name),
			}
		}
		if err := addToolParameterHeadersForBindings(headers, bindings, params["arguments"]); err != nil {
			return nil, err
		}
	}
	return headers, nil
}

func validateProtocolMeta(raw json.RawMessage) error {
	meta, err := decodeObjectFields(raw)
	if err != nil {
		return &InvalidPayloadError{Subject: subjectMCPRequestMeta, Err: err}
	}
	var version string
	if err := json.Unmarshal(meta["io.modelcontextprotocol/protocolVersion"], &version); err != nil {
		return &InvalidPayloadError{
			Subject: "MCP request protocol version",
			Err:     errors.New("missing or invalid value"),
		}
	}
	if version != ProtocolVersion {
		return &InvalidPayloadError{
			Subject: "MCP request protocol version",
			Err:     fmt.Errorf("body version %q does not match %q", version, ProtocolVersion),
		}
	}
	return nil
}

func (t *StreamableHTTPTransport) addToolParameterHeaders(
	headers map[string]string,
	tool string,
	arguments json.RawMessage,
) error {
	t.mu.RLock()
	bindings := append([]HTTPToolHeaderBinding(nil), t.toolHeaders[tool]...)
	t.mu.RUnlock()
	return addToolParameterHeadersForBindings(headers, bindings, arguments)
}

func addToolParameterHeadersForBindings(
	headers map[string]string,
	bindings []HTTPToolHeaderBinding,
	arguments json.RawMessage,
) error {
	if len(bindings) == 0 {
		return nil
	}
	if len(bytes.TrimSpace(arguments)) == 0 || rawJSONIsNull(arguments) {
		return nil
	}
	root, err := decodeObjectFields(arguments)
	if err != nil {
		return &InvalidPayloadError{Subject: "tools/call arguments", Err: err}
	}
	for _, binding := range bindings {
		value, found, err := extractHeaderPath(root, binding.Path)
		if err != nil {
			return err
		}
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		encoded, err := encodeTypedHeaderValue(value, binding.Type)
		if err != nil {
			return &InvalidPayloadError{
				Subject: mcpParamHeaderPrefix + binding.Header,
				Err:     err,
			}
		}
		headers[mcpParamHeaderPrefix+binding.Header] = encodeMCPHeaderValue(encoded)
	}
	return nil
}

func extractHeaderPath(root map[string]json.RawMessage, path []string) (json.RawMessage, bool, error) {
	current := root
	for index, segment := range path {
		value, ok := current[segment]
		if !ok {
			return nil, false, nil
		}
		if index == len(path)-1 {
			return value, true, nil
		}
		next, err := decodeObjectFields(value)
		if err != nil {
			return nil, false, fmt.Errorf(
				"mcp: x-mcp-header path %q is not an object",
				strings.Join(path[:index+1], "."),
			)
		}
		current = next
	}
	return nil, false, nil
}

func encodeTypedHeaderValue(raw json.RawMessage, expectedType string) (string, error) {
	switch expectedType {
	case schemaTypeString:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", errors.New("value must be a string")
		}
		return value, nil
	case schemaTypeBoolean:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", errors.New("value must be a boolean")
		}
		return strconv.FormatBool(value), nil
	case schemaTypeInteger:
		var value json.Number
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil || !isIntegralJSONNumber(JSONNumber(value.String())) {
			return "", errors.New("value must be an integer")
		}
		var rational big.Rat
		if _, ok := rational.SetString(value.String()); !ok || !rational.IsInt() {
			return "", errors.New("value must be an integer")
		}
		integer := rational.Num()
		limit := big.NewInt(maxSafeInteger)
		if integer.Cmp(limit) > 0 || integer.Cmp(new(big.Int).Neg(limit)) < 0 {
			return "", errors.New("integer value is outside the JavaScript safe range")
		}
		return integer.String(), nil
	default:
		return "", fmt.Errorf("unsupported primitive type %q", expectedType)
	}
}

func encodeMCPHeaderValue(value string) string {
	safe := strings.Trim(value, " \t") == value &&
		(!strings.HasPrefix(value, "=?base64?") || !strings.HasSuffix(value, "?="))
	for index := 0; safe && index < len(value); index++ {
		char := value[index]
		safe = char >= ' ' && char <= '~'
	}
	if safe {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func requestMethod(body []byte) (string, error) {
	var request struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(body, &request); err != nil || request.Method == "" {
		return "", &InvalidPayloadError{Subject: streamableHTTPRequestSubject, Err: errors.New("method is required")}
	}
	return request.Method, nil
}

type sseRequestProvenance struct {
	requestID     json.RawMessage
	method        string
	progressToken json.RawMessage
	hasProgress   bool
	logging       bool
	acknowledged  bool
}

func newSSERequestProvenance(body []byte) (*sseRequestProvenance, error) {
	fields, err := decodeObjectFields(body)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: streamableHTTPRequestSubject, Err: err}
	}
	id, hasID := fields["id"]
	if !hasID {
		return nil, &InvalidPayloadError{
			Subject: streamableHTTPRequestSubject,
			Err:     errors.New("request id is required"),
		}
	}
	if _, keyErr := rpcIDKey(id); keyErr != nil {
		return nil, &InvalidPayloadError{Subject: streamableHTTPRequestSubject, Err: keyErr}
	}
	method, err := requestMethod(body)
	if err != nil {
		return nil, err
	}
	params, err := decodeObjectFields(fields["params"])
	if err != nil {
		return nil, &InvalidPayloadError{Subject: "Streamable HTTP params", Err: err}
	}
	meta, err := decodeObjectFields(params["_meta"])
	if err != nil {
		return nil, &InvalidPayloadError{Subject: subjectMCPRequestMeta, Err: err}
	}
	progressToken, hasProgress := meta[progressTokenField]
	if hasProgress {
		if _, err := notificationCorrelationKey(progressToken); err != nil {
			return nil, &InvalidPayloadError{Subject: "MCP progress token", Err: err}
		}
	}
	_, logging := meta[metaLogLevel]
	return &sseRequestProvenance{
		requestID:     bytes.Clone(id),
		method:        method,
		progressToken: bytes.Clone(progressToken),
		hasProgress:   hasProgress,
		logging:       logging,
	}, nil
}

func exactMediaType(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(value)
	return mediaType, err
}

func validateJSONPostCorrelation(requestBody, responseBody []byte) error {
	requestFields, err := decodeObjectFields(requestBody)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPRequestSubject, Err: err}
	}
	responseFields, err := decodeObjectFields(responseBody)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	if _, hasMethod := responseFields["method"]; hasMethod {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("terminal response required"),
		}
	}
	requestKey, err := rpcIDKey(requestFields["id"])
	if err != nil {
		return &InvalidPayloadError{Subject: "Streamable HTTP request id", Err: err}
	}
	responseKey, err := rpcIDKey(responseFields["id"])
	if err != nil {
		return &InvalidPayloadError{Subject: "Streamable HTTP response id", Err: err}
	}
	if requestKey != responseKey {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("response id does not match request id"),
		}
	}
	return nil
}

func validateNon2xxRPCError(status int, requestBody, responseBody []byte) error {
	if err := validateJSONPostCorrelation(requestBody, responseBody); err != nil {
		return err
	}
	fields, err := decodeObjectFields(responseBody)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	var version string
	versionErr := json.Unmarshal(fields["jsonrpc"], &version)
	if versionErr != nil || version != JSONRPCVersion {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("valid jsonrpc version is required"),
		}
	}
	_, hasError := fields["error"]
	_, hasResult := fields["result"]
	_, hasParams := fields["params"]
	if !hasError || hasResult || hasParams {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("non-success HTTP status requires a JSON-RPC error response"),
		}
	}
	errorValidationErr := validateJSONRPCError(fields["error"])
	if errorValidationErr != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: errorValidationErr}
	}
	reserved, err := hasReservedHTTP400RPCError(fields)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	if reserved && status != http.StatusBadRequest {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     fmt.Errorf("reserved MCP error requires HTTP 400, got %d", status),
		}
	}
	if reserved {
		if err := validateReservedMCPRPCError(fields["error"]); err != nil {
			return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
		}
	}
	return nil
}

func validateReservedMCPRPCError(raw json.RawMessage) error {
	var wireError JSONRPCError
	if err := json.Unmarshal(raw, &wireError); err != nil {
		return err
	}
	mapped := TypedRPCError(&RPCError{
		Code:    wireError.Code,
		Message: wireError.Message,
		Data:    bytes.Clone(wireError.Data),
	})
	if invalid, ok := errors.AsType[*InvalidPayloadError](mapped); ok {
		return invalid
	}
	return nil
}

func rejectReservedRPCErrorOutsideHTTP400(responseBody []byte) error {
	fields, err := decodeObjectFields(responseBody)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	reserved, err := hasReservedHTTP400RPCError(fields)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	if !reserved {
		return nil
	}
	return &InvalidPayloadError{
		Subject: streamableHTTPJSONResponseSubject,
		Err:     errors.New("reserved MCP error requires HTTP 400, got successful HTTP status"),
	}
}

func hasReservedHTTP400RPCError(fields map[string]json.RawMessage) (bool, error) {
	raw, hasError := fields["error"]
	if !hasError {
		return false, nil
	}
	errorFields, err := decodeObjectFields(raw)
	if err != nil {
		return false, err
	}
	var code JSONNumber
	if err := json.Unmarshal(errorFields["code"], &code); err != nil {
		return false, err
	}
	switch code {
	case JSONRPCHeaderMismatch, JSONRPCMissingRequiredClientCapability, JSONRPCUnsupportedProtocolVersion:
		return true, nil
	case JSONRPCParseError,
		JSONRPCInvalidRequest,
		JSONRPCMethodNotFound,
		JSONRPCInvalidParams,
		JSONRPCInternalError:
		return false, nil
	default:
		return false, nil
	}
}

//nolint:funlen,gocognit // SSE framing and terminal correlation are kept in one fail-closed parser.
func (t *StreamableHTTPTransport) consumeRequestSSE(
	ctx context.Context,
	reader io.Reader,
	provenance *sseRequestProvenance,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	limited := reader
	if t.limits.MaxLifetimeBytes > 0 {
		limited = httptool.LimitStreamReaderWithContext(ctx, reader, t.limits.MaxLifetimeBytes)
	}
	scanner := bufio.NewScanner(limited)
	// Scanner headroom accepts an inclusive frame plus line delimiters.
	// Oversized tokens are mapped to the same typed frame-limit cause.
	scannerLimit := t.maxFrameBytes + 1
	if scannerLimit < math.MaxInt {
		scannerLimit++
	}
	scanner.Buffer(nil, scannerLimit)
	lineBytesConsumed := 0
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		advance, token := splitSSELines(data, atEOF)
		if token != nil {
			lineBytesConsumed = advance
		}
		return advance, token, nil
	})
	frameBytes := 0
	var data strings.Builder
	dataSeen := false
	terminal := false
	dispatch := func() error {
		defer data.Reset()
		if !dataSeen {
			return nil
		}
		dataSeen = false
		if data.Len() == 0 {
			return nil
		}
		payload := []byte(data.String())
		isTerminal, acknowledged, err := validateSSEProvenance(payload, provenance)
		if err != nil {
			return err
		}
		if terminal {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("message after terminal response"),
			}
		}
		handler := t.requestNotificationHandler(payload, provenance.requestID)
		if err := t.peer.dispatchWithNotificationHandler(payload, handler); err != nil {
			return err
		}
		if acknowledged {
			provenance.acknowledged = true
		}
		terminal = isTerminal
		return nil
	}
	firstLine := true
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if lineBytesConsumed > t.maxFrameBytes-frameBytes {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     frameLimitError("SSE frame", t.maxFrameBytes),
			}
		}
		frameBytes += lineBytesConsumed
		lineBytes := scanner.Bytes()
		if firstLine {
			lineBytes = bytes.TrimPrefix(lineBytes, []byte{0xef, 0xbb, 0xbf})
			firstLine = false
		}
		if !utf8.Valid(lineBytes) {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("event stream is not valid UTF-8"),
			}
		}
		line := string(lineBytes)
		if line == "" {
			frameBytes = 0
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, hasColon := strings.Cut(line, ":")
		if !hasColon {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		if field == "data" {
			if dataSeen {
				data.WriteByte('\n')
			}
			dataSeen = true
			data.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			err = frameLimitError("SSE frame", t.maxFrameBytes)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: err}
	}
	// EOF is not an SSE event boundary. Pending data has no terminating
	// empty line and must not complete a request or invoke a handler.
	if !terminal {
		return &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("stream ended before terminal response"),
		}
	}
	return nil
}

func (t *StreamableHTTPTransport) requestNotificationHandler(
	payload []byte,
	requestID json.RawMessage,
) NotificationHandler {
	fields, err := decodeObjectFields(payload)
	if err != nil {
		return nil
	}
	var method string
	if err := json.Unmarshal(fields["method"], &method); err != nil || method == "" {
		return nil
	}
	t.mu.RLock()
	handler := t.requestHandlers[method]
	t.mu.RUnlock()
	if handler == nil {
		return nil
	}
	originID := bytes.Clone(requestID)
	return func(params json.RawMessage) { handler(bytes.Clone(originID), params) }
}

func validateSSEProvenance(
	payload []byte,
	provenance *sseRequestProvenance,
) (bool, bool, error) {
	fields, err := decodeObjectFields(payload)
	if err != nil {
		return false, false, &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: err}
	}
	if _, hasMethod := fields["method"]; hasMethod {
		if _, hasID := fields["id"]; hasID {
			return false, false, &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("server-initiated request is forbidden"),
			}
		}
		var method string
		if unmarshalErr := json.Unmarshal(fields["method"], &method); unmarshalErr != nil ||
			!allowedSSEMethod(provenance.method, method) {
			return false, false, &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     fmt.Errorf("notification is not valid for request %q", provenance.method),
			}
		}
		if validationErr := validateSSENotificationParams(fields["params"], method, provenance); validationErr != nil {
			return false, false, validationErr
		}
		return false, method == MethodSubscriptionsAcknowledged, nil
	}
	key, err := rpcIDKey(fields["id"])
	if err != nil {
		return false, false, &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("terminal response id is required"),
		}
	}
	expected, _ := rpcIDKey(provenance.requestID)
	if key != expected {
		return false, false, &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("response id does not match request stream"),
		}
	}
	return validateSSETerminal(payload, fields, provenance)
}

func validateSSETerminal(
	payload []byte,
	fields map[string]json.RawMessage,
	provenance *sseRequestProvenance,
) (bool, bool, error) {
	if provenance.method == MethodSubscriptionsListen {
		return validateSubscriptionSSETerminal(payload, fields, provenance)
	}
	if err := rejectReservedRPCErrorOutsideHTTP400(payload); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func validateSubscriptionSSETerminal(
	payload []byte,
	fields map[string]json.RawMessage,
	provenance *sseRequestProvenance,
) (bool, bool, error) {
	if _, hasError := fields["error"]; hasError {
		if err := rejectReservedRPCErrorOutsideHTTP400(payload); err != nil {
			return false, false, err
		}
		return true, false, nil
	}
	if !provenance.acknowledged {
		return false, false, &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("subscription terminated before acknowledgement"),
		}
	}
	if err := validateSubscriptionTerminal(fields["result"], provenance.requestID); err != nil {
		return false, false, err
	}
	return true, false, nil
}

//nolint:gocognit // Stream-specific provenance is validated in one pre-dispatch boundary.
func validateSSENotificationParams(
	raw json.RawMessage,
	method string,
	provenance *sseRequestProvenance,
) error {
	params, err := decodeObjectFields(raw)
	if err != nil {
		return &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("notification params are required"),
		}
	}
	if provenance.method == MethodSubscriptionsListen {
		meta, metaErr := decodeObjectFields(params["_meta"])
		if metaErr != nil {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("subscription metadata is required"),
			}
		}
		if err := matchRPCID(meta[metaSubscriptionID], provenance.requestID); err != nil {
			return &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: fmt.Errorf("subscription id: %w", err)}
		}
		if !provenance.acknowledged && method != MethodSubscriptionsAcknowledged {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("subscription acknowledgement must be first"),
			}
		}
		if provenance.acknowledged && method == MethodSubscriptionsAcknowledged {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("duplicate subscription acknowledgement"),
			}
		}
		return nil
	}
	if metaRaw, hasMeta := params["_meta"]; hasMeta {
		meta, metaErr := decodeObjectFields(metaRaw)
		if metaErr != nil {
			return &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: metaErr}
		}
		if _, hasSubscriptionID := meta[metaSubscriptionID]; hasSubscriptionID {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("subscription id on request-scoped notification"),
			}
		}
	}
	switch method {
	case MethodProgress:
		if !provenance.hasProgress {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("unsolicited progress notification"),
			}
		}
		expected, _ := notificationCorrelationKey(provenance.progressToken)
		actual, keyErr := notificationCorrelationKey(params[progressTokenField])
		if keyErr != nil || actual != expected {
			return &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: errors.New("progress token mismatch")}
		}
	case MethodLogMessage:
		if !provenance.logging {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("unsolicited logging notification"),
			}
		}
	}
	return nil
}

func validateSubscriptionTerminal(raw, requestID json.RawMessage) error {
	if len(raw) == 0 {
		return nil // Error responses do not carry subscription result metadata.
	}
	result, err := decodeObjectFields(raw)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: err}
	}
	meta, err := decodeObjectFields(result["_meta"])
	if err != nil {
		return &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     errors.New("subscription terminal metadata is required"),
		}
	}
	if err := matchRPCID(meta[metaSubscriptionID], requestID); err != nil {
		return &InvalidPayloadError{
			Subject: streamableHTTPSSESubject,
			Err:     fmt.Errorf("terminal subscription id: %w", err),
		}
	}
	return nil
}

func matchRPCID(actual, expected json.RawMessage) error {
	actualKey, err := rpcIDKey(actual)
	if err != nil {
		return errors.New("missing or invalid value")
	}
	expectedKey, _ := rpcIDKey(expected)
	if actualKey != expectedKey {
		return errors.New("value does not match request id")
	}
	return nil
}

func notificationCorrelationKey(raw json.RawMessage) (string, error) {
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
		canonical, ok := canonicalJSONNumber(typed.String())
		if !ok {
			return "", errors.New("token must be a string or number")
		}
		return "n:" + canonical, nil
	default:
		return "", errors.New("token must be a string or number")
	}
}

func allowedSSEMethod(originMethod, notificationMethod string) bool {
	if originMethod == MethodSubscriptionsListen {
		switch notificationMethod {
		case "notifications/subscriptions/acknowledged", "notifications/tools/list_changed",
			"notifications/resources/list_changed",
			"notifications/resources/updated", "notifications/prompts/list_changed":
			return true
		default:
			return false
		}
	}
	return notificationMethod == "notifications/progress" || notificationMethod == "notifications/message"
}

func splitSSELines(data []byte, atEOF bool) (int, []byte) {
	for index, char := range data {
		switch char {
		case '\n':
			return index + 1, data[:index]
		case '\r':
			if index+1 == len(data) && !atEOF {
				return 0, nil
			}
			advance := index + 1
			if index+1 < len(data) && data[index+1] == '\n' {
				advance++
			}
			return advance, data[:index]
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data
	}
	return 0, nil
}

func isTerminalHTTPTransportError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if _, ok := errors.AsType[*InvalidPayloadError](err); ok {
		return true
	}
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode < http.StatusInternalServerError
}

func (t *StreamableHTTPTransport) terminate(cause error) {
	t.mu.Lock()
	if t.closed || t.terminating {
		t.mu.Unlock()
		return
	}
	t.terminalErr = cause
	t.terminating = true
	cancel := t.cancel
	peer := t.peer
	cancelPosts := make([]context.CancelFunc, 0, len(t.activePosts))
	for _, cancelPost := range t.activePosts {
		cancelPosts = append(cancelPosts, cancelPost)
	}
	t.mu.Unlock()
	if peer != nil {
		peer.close(cause)
	}
	for _, cancelPost := range cancelPosts {
		cancelPost()
	}
	if cancel != nil {
		cancel()
	}
	go func() { _ = t.Close() }()
}

func (t *StreamableHTTPTransport) MaxFrameBytes() int { return t.maxFrameBytes }

func (t *StreamableHTTPTransport) failureCause(fallback error) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.terminalErr != nil {
		return t.terminalErr
	}
	return fallback
}

func (t *StreamableHTTPTransport) Close() error {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return
		}
		if t.terminalErr == nil {
			t.terminalErr = ErrTransportClosed
		}
		t.closed = true
		cancelPosts := make([]context.CancelFunc, 0, len(t.activePosts))
		for _, cancelPost := range t.activePosts {
			cancelPosts = append(cancelPosts, cancelPost)
		}
		cancel := t.cancel
		peer := t.peer
		t.mu.Unlock()
		if peer != nil {
			peer.close(ErrTransportClosed)
		}
		for _, cancelPost := range cancelPosts {
			cancelPost()
		}
		if cancel != nil {
			cancel()
		}
		t.postWG.Wait()
		if t.client != nil {
			t.client.CloseIdleConnections()
		}
	})
	return nil
}

var _ Transport = (*StreamableHTTPTransport)(nil)
var _ ToolHeaderTransport = (*StreamableHTTPTransport)(nil)
var _ FrameByteCapTransport = (*StreamableHTTPTransport)(nil)
