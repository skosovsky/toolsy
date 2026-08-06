//nolint:exhaustruct // Transport options deliberately initialize only configured runtime state.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	httpPollRetryDelay = 100 * time.Millisecond
	httpMaxRetryDelay  = 5 * time.Minute
)

const (
	mcpProtocolVersionHeader          = "Mcp-Protocol-Version"
	mcpSessionIDHeader                = "Mcp-Session-Id"
	httpCloseTimeout                  = 5 * time.Second
	eventStreamMediaType              = "text/event-stream"
	streamableHTTPResponseSubject     = "Streamable HTTP response"
	streamableHTTPSSESubject          = "Streamable HTTP SSE"
	streamableHTTPJSONResponseSubject = "Streamable HTTP JSON response"
)

var errSSEPollingBoundary = errors.New("mcp: SSE polling boundary")

type sseResumeState struct {
	lastEventID string
	retryDelay  time.Duration
}

type sseBoundaryError struct{ state sseResumeState }

func (e *sseBoundaryError) Error() string { return errSSEPollingBoundary.Error() }
func (e *sseBoundaryError) Unwrap() error { return errSSEPollingBoundary }

type HTTPRequestDecorator func(*http.Request) error

type StreamableHTTPOption func(*StreamableHTTPTransport)

func WithStreamableHTTPLogger(logger *slog.Logger) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) {
		if logger != nil {
			transport.logger = logger
		}
	}
}

func WithStreamableHTTPMaxStreamBytes(limit int) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) {
		if limit > 0 {
			transport.maxStreamBytes = limit
		}
	}
}

func WithStreamableHTTPAllowPrivateIPs(allow bool) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) {
		transport.allowPrivateIPs = allow
	}
}

// WithStreamableHTTPClient applies safe timeout settings from client. Its Transport is
// intentionally ignored so SSRF-safe dialing and redirect validation remain enforced.
func WithStreamableHTTPClient(client *http.Client) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) {
		transport.baseClient = client
	}
}

func WithStreamableHTTPRequestDecorator(decorator HTTPRequestDecorator) StreamableHTTPOption {
	return func(transport *StreamableHTTPTransport) { transport.decorator = decorator }
}

func defaultStreamableHTTPClient(allowPrivateIPs bool) *http.Client {
	validateRedirect := httptool.CheckRedirectRemote(allowPrivateIPs, nil)
	client := httptool.NewSafeHTTPClient(
		httptool.SafeDialOptions{
			AllowPrivateIPs: allowPrivateIPs,
		}, //nolint:exhaustruct // blacklist mode
		func(request *http.Request, via []*http.Request) error {
			if len(via) > 0 && request.Method != via[len(via)-1].Method {
				return errors.New("mcp: redirect must preserve HTTP method")
			}
			return validateRedirect(request, via)
		},
	)
	client.Timeout = 0
	return client
}

type StreamableHTTPTransport struct {
	endpoint   string
	logger     *slog.Logger
	client     *http.Client
	baseClient *http.Client
	decorator  HTTPRequestDecorator

	allowPrivateIPs bool
	maxStreamBytes  int
	mu              sync.RWMutex
	started         bool
	closed          bool
	terminating     bool
	terminalErr     error
	lifetimeCtx     context.Context
	cancel          context.CancelFunc
	peer            *rpcPeer
	protocolVersion string
	sessionID       string
	lastEventID     string
	getStarted      bool
	getDone         chan struct{}
	pollRetryDelay  time.Duration
	requestHandler  RequestHandler
	notifyHandlers  map[string]NotificationHandler
	activePosts     map[uint64]context.CancelFunc
	nextPostID      uint64
	postWG          sync.WaitGroup
	resumeWG        sync.WaitGroup
	closeOnce       sync.Once
}

func NewStreamableHTTPTransport(
	endpoint string,
	opts ...StreamableHTTPOption,
) *StreamableHTTPTransport {
	transport := &StreamableHTTPTransport{
		endpoint:       endpoint,
		logger:         slog.Default(),
		client:         defaultStreamableHTTPClient(false),
		maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
		getDone:        make(chan struct{}),
		pollRetryDelay: httpPollRetryDelay,
		notifyHandlers: make(map[string]NotificationHandler),
		activePosts:    make(map[uint64]context.CancelFunc),
	}
	for _, opt := range opts {
		opt(transport)
	}
	transport.client = httptool.MergeHTTPClient(
		defaultStreamableHTTPClient(transport.allowPrivateIPs),
		transport.baseClient,
	)
	return transport
}

func (t *StreamableHTTPTransport) Start(ctx context.Context) error {
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
	t.peer = newRPCPeer(t.lifetimeCtx, t.logger, t.postMessage)
	t.peer.setRequestHandler(t.requestHandler)
	for method, handler := range t.notifyHandlers {
		t.peer.setNotificationHandler(method, handler)
	}
	t.started = true
	return nil
}

func (t *StreamableHTTPTransport) SetProtocolVersion(version string) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.protocolVersion = version
	t.mu.Unlock()
}

func (t *StreamableHTTPTransport) Activate() { t.ensureGETStarted() }

func (t *StreamableHTTPTransport) ensureGETStarted() {
	t.mu.Lock()
	if t.getStarted || !t.started || t.closed || t.terminating {
		t.mu.Unlock()
		return
	}
	t.getStarted = true
	t.mu.Unlock()
	go t.getLoop()
}

func (t *StreamableHTTPTransport) Request(
	ctx context.Context,
	method string,
	params any,
) (PendingRequest, error) {
	if err := ctx.Err(); err != nil {
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
	go func() {
		postCtx, stopPost := contextUntilPendingTerminal(ctx, pending.terminal)
		defer stopPost()
		defer pending.finishDelivery()
		if postErr := t.postMessageTracked(postCtx, body, pending.markSent); postErr != nil {
			peer.failPending(pending, t.failureCause(postErr))
		}
	}()
	return pending, nil
}

func (t *StreamableHTTPTransport) Notify(ctx context.Context, method string, params any) error {
	t.mu.RLock()
	peer := t.peer
	started := t.started && !t.closed && !t.terminating
	t.mu.RUnlock()
	if peer == nil || !started {
		return ErrTransportClosed
	}
	return peer.notifyMessage(ctx, method, params)
}

func (t *StreamableHTTPTransport) OnRequest(handler RequestHandler) {
	t.mu.Lock()
	t.requestHandler = handler
	peer := t.peer
	t.mu.Unlock()
	if peer != nil {
		peer.setRequestHandler(handler)
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

func (t *StreamableHTTPTransport) buildRequest(
	ctx context.Context,
	method string,
	body io.Reader,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, t.endpoint, nil)
	if err != nil {
		return nil, err
	}
	t.mu.RLock()
	version := t.protocolVersion
	sessionID := t.sessionID
	lastEventID := t.lastEventID
	decorator := t.decorator
	t.mu.RUnlock()
	if decorator != nil {
		if decoratorErr := applyRequestDecorator(request, decorator); decoratorErr != nil {
			return nil, decoratorErr
		}
	}
	decoratedHeaders := request.Header.Clone()
	request, err = http.NewRequestWithContext(ctx, method, t.endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header = decoratedHeaders
	request.Header.Set("Accept", "application/json, text/event-stream")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if version != "" {
		request.Header.Set(mcpProtocolVersionHeader, version)
	}
	if sessionID != "" {
		request.Header.Set(mcpSessionIDHeader, sessionID)
	}
	if method == http.MethodGet && lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	return request, nil
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
			Subject: "HTTP request decorator",
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
		Subject: "HTTP request decorator",
		Err:     errors.New("decorator must not set request body fields"),
	}
}

func requestDecoratorChangedBody(request *http.Request) bool {
	return request.Body != nil || request.GetBody != nil || request.ContentLength != 0 ||
		len(request.TransferEncoding) > 0 || len(request.Trailer) > 0 ||
		request.Header.Get("Content-Length") != "" ||
		request.Header.Get("Transfer-Encoding") != "" ||
		request.Header.Get("Trailer") != ""
}

func (t *StreamableHTTPTransport) postMessage(ctx context.Context, body []byte) error {
	return t.postMessageTracked(ctx, body, nil)
}

func (t *StreamableHTTPTransport) postMessageTracked(
	ctx context.Context,
	body []byte,
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
	t.nextPostID++
	postID := t.nextPostID
	t.activePosts[postID] = cancelPost
	t.postWG.Add(1)
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.activePosts, postID)
		t.mu.Unlock()
		cancelPost()
		t.postWG.Done()
	}()
	err := t.postMessageOnce(postCtx, body, onSent)
	if isTerminalHTTPTransportError(err) {
		go t.terminate(err)
	}
	return err
}

//nolint:funlen,gocognit // HTTP status, session, JSON and SSE branches are explicit fail-closed paths.
func (t *StreamableHTTPTransport) postMessageOnce(
	ctx context.Context,
	body []byte,
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
	request, err := t.buildRequest(ctx, http.MethodPost, bytes.NewReader(body))
	if err != nil {
		return err
	}
	// #nosec G704 -- endpoint validation and safe dialing are enforced in Start/default client.
	//nolint:bodyclose // Closed through httptool.CloseResponseBody below.
	response, err := t.client.Do(
		request,
	)
	if err != nil {
		return err
	}
	defer httptool.CloseResponseBody(ctx, response.Body)
	if response.StatusCode == http.StatusNotFound && t.hasSession() {
		return ErrSessionExpired
	}
	if response.StatusCode == http.StatusUnauthorized ||
		response.StatusCode == http.StatusForbidden {
		return &HTTPError{StatusCode: response.StatusCode, Operation: "POST authentication"}
	}
	if response.StatusCode == http.StatusAccepted {
		if isJSONRPCRequest(body) {
			return &InvalidPayloadError{
				Subject: streamableHTTPResponseSubject,
				Err:     errors.New("202 Accepted is invalid for a JSON-RPC request"),
			}
		}
		payload, readErr := httptool.ReadBodyLimited(ctx, response.Body, t.maxStreamBytes)
		if readErr != nil {
			return readErr
		}
		if len(payload) != 0 {
			return &InvalidPayloadError{
				Subject: "Streamable HTTP 202 response",
				Err:     errors.New("202 Accepted response body must be empty"),
			}
		}
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPError{StatusCode: response.StatusCode, Operation: "POST"}
	}
	if !isJSONRPCRequest(body) {
		return &InvalidPayloadError{
			Subject: streamableHTTPResponseSubject,
			Err:     errors.New("notification and response POSTs require 202 Accepted"),
		}
	}
	sessionHeader := response.Header.Get(mcpSessionIDHeader)
	if sessionErr := t.acceptSessionHeader(sessionHeader, isInitializeRequest(body)); sessionErr != nil {
		return sessionErr
	}
	contentType, err := exactMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return &InvalidPayloadError{Subject: "Streamable HTTP Content-Type", Err: err}
	}
	switch contentType {
	case "application/json":
		payload, err := httptool.ReadBodyLimited(ctx, response.Body, t.maxStreamBytes)
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			return &InvalidPayloadError{
				Subject: streamableHTTPJSONResponseSubject,
				Err:     errors.New("empty body"),
			}
		}
		if err := validateJSONPostCorrelation(body, payload); err != nil {
			return err
		}
		return t.peer.dispatch(payload)
	case eventStreamMediaType:
		defaults := t.snapshotResumeState()
		initial := sseResumeState{retryDelay: defaults.retryDelay}
		_, err := t.consumeSSEState(ctx, response.Body, initial, false)
		var boundary *sseBoundaryError
		if errors.As(err, &boundary) {
			if requestID, ok := jsonRPCRequestID(body); ok && t.peer.hasPendingID(requestID) {
				t.startResumeLoop(boundary.state, requestID)
			}
			return nil
		}
		return err
	default:
		return &InvalidPayloadError{
			Subject: streamableHTTPResponseSubject,
			Err:     fmt.Errorf("unsupported content type %q", contentType),
		}
	}
}

func exactMediaType(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return "", err
	}
	return mediaType, nil
}

func validateJSONPostCorrelation(requestBody, responseBody []byte) error {
	if !isJSONRPCRequest(requestBody) {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("JSON response is only valid for a JSON-RPC request"),
		}
	}
	requestFields, err := decodeObjectFields(requestBody)
	if err != nil {
		return &InvalidPayloadError{Subject: "Streamable HTTP request", Err: err}
	}
	responseFields, err := decodeObjectFields(responseBody)
	if err != nil {
		return &InvalidPayloadError{Subject: streamableHTTPJSONResponseSubject, Err: err}
	}
	if _, hasMethod := responseFields["method"]; hasMethod {
		return &InvalidPayloadError{
			Subject: streamableHTTPJSONResponseSubject,
			Err:     errors.New("POST request requires a terminal JSON-RPC response"),
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
			Err:     errors.New("response id does not match POST request id"),
		}
	}
	return nil
}

func isTerminalHTTPTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSessionExpired) {
		return true
	}
	var invalid *InvalidPayloadError
	if errors.As(err, &invalid) {
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
	go func() {
		_ = t.Close()
	}()
}

func (t *StreamableHTTPTransport) getLoop() {
	defer close(t.getDone)
	state := t.snapshotResumeState()
	for {
		if err := t.lifetimeCtx.Err(); err != nil {
			return
		}
		var err error
		state, err = t.pollGET(t.lifetimeCtx, state, true)
		var boundary *sseBoundaryError
		if errors.As(err, &boundary) {
			state = boundary.state
			t.storeResumeState(state)
			err = nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrTransportClosed) {
			return
		}
		if errors.Is(err, ErrSessionExpired) {
			t.terminate(err)
			return
		}
		if errors.Is(err, errGETNotSupported) {
			return
		}
		var invalid *InvalidPayloadError
		var httpErr *HTTPError
		if errors.As(err, &invalid) ||
			(errors.As(err, &httpErr) && httpErr.StatusCode < http.StatusInternalServerError) {
			t.terminate(err)
			return
		}
		if err != nil {
			t.logger.Warn("mcp: Streamable HTTP GET poll", "err", err)
		}
		if !waitForRetry(t.lifetimeCtx, state.retryDelay) {
			return
		}
	}
}

func (t *StreamableHTTPTransport) startResumeLoop(
	state sseResumeState,
	requestID json.RawMessage,
) {
	pendingDone, ok := t.peer.pendingDone(requestID)
	if !ok {
		return
	}
	t.mu.Lock()
	if t.closed || t.terminating {
		t.mu.Unlock()
		return
	}
	t.resumeWG.Add(1)
	t.mu.Unlock()
	go func() {
		defer t.resumeWG.Done()
		t.resumeLoop(state, bytes.Clone(requestID), pendingDone)
	}()
}

//nolint:gocognit // Resume termination and retry outcomes are intentionally explicit.
func (t *StreamableHTTPTransport) resumeLoop(
	state sseResumeState,
	requestID json.RawMessage,
	pendingDone <-chan struct{},
) {
	resumeCtx, cancelResume := context.WithCancel(t.lifetimeCtx)
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-pendingDone:
			cancelResume()
		case <-resumeCtx.Done():
		}
	}()
	defer func() {
		cancelResume()
		<-watcherDone
	}()
	for {
		if !t.peer.hasPendingID(requestID) {
			return
		}
		if !waitForRetry(resumeCtx, state.retryDelay) {
			return
		}
		next, err := t.pollGET(resumeCtx, state, false)
		var boundary *sseBoundaryError
		if errors.As(err, &boundary) {
			state = boundary.state
			continue
		}
		if !t.peer.hasPendingID(requestID) {
			return
		}
		state = next
		if errors.Is(err, errGETNotSupported) {
			t.peer.failPendingID(
				requestID,
				&HTTPError{StatusCode: http.StatusMethodNotAllowed, Operation: "SSE resume GET"},
			)
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrTransportClosed) {
			return
		}
		if errors.Is(err, ErrSessionExpired) {
			t.terminate(err)
			return
		}
		var invalid *InvalidPayloadError
		var httpErr *HTTPError
		if errors.As(err, &invalid) ||
			(errors.As(err, &httpErr) && httpErr.StatusCode < http.StatusInternalServerError) {
			t.terminate(err)
			return
		}
		if err != nil {
			t.logger.Warn("mcp: Streamable HTTP resume", "err", err)
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay < httpPollRetryDelay {
		delay = httpPollRetryDelay
	} else if delay > httpMaxRetryDelay {
		delay = httpMaxRetryDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var errGETNotSupported = errors.New("mcp: Streamable HTTP GET not supported")

func (t *StreamableHTTPTransport) pollGET(
	ctx context.Context,
	state sseResumeState,
	persist bool,
) (sseResumeState, error) {
	request, err := t.buildRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return state, err
	}
	request.Header.Set("Accept", eventStreamMediaType)
	request.Header.Del("Last-Event-ID")
	if state.lastEventID != "" {
		request.Header.Set("Last-Event-ID", state.lastEventID)
	}
	// #nosec G704 -- endpoint validation and safe dialing are enforced in Start/default client.
	//nolint:bodyclose // Closed through httptool.CloseResponseBody below.
	response, err := t.client.Do(
		request,
	)
	if err != nil {
		return state, err
	}
	defer httptool.CloseResponseBody(ctx, response.Body)
	if response.StatusCode == http.StatusMethodNotAllowed {
		return state, errGETNotSupported
	}
	if response.StatusCode == http.StatusNotFound && t.hasSession() {
		return state, ErrSessionExpired
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return state, &HTTPError{StatusCode: response.StatusCode, Operation: "GET"}
	}
	contentType, mediaErr := exactMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || contentType != eventStreamMediaType {
		return state, &InvalidPayloadError{
			Subject: "Streamable HTTP GET",
			Err:     errors.New("response Content-Type is not text/event-stream"),
		}
	}
	return t.consumeSSEState(ctx, response.Body, state, persist)
}

func (t *StreamableHTTPTransport) consumeSSE(
	ctx context.Context,
	reader io.Reader,
) error {
	state, err := t.consumeSSEState(ctx, reader, t.snapshotResumeState(), true)
	t.storeResumeState(state)
	return err
}

//nolint:funlen,gocognit // SSE field folding is intentionally explicit.
func (t *StreamableHTTPTransport) consumeSSEState(
	ctx context.Context,
	reader io.Reader,
	state sseResumeState,
	persist bool,
) (sseResumeState, error) {
	limited := httptool.LimitStreamReaderWithContext(ctx, reader, t.maxStreamBytes)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(nil, t.maxStreamBytes)
	scanner.Split(splitSSELines)
	var data strings.Builder
	var eventType, eventID string
	var eventIDSeen, dataSeen bool
	dispatch := func() error {
		defer data.Reset()
		if eventIDSeen {
			if strings.ContainsAny(eventID, "\x00\r\n") {
				return &InvalidPayloadError{
					Subject: "Streamable HTTP SSE event id",
					Err:     errors.New("contains forbidden characters"),
				}
			}
			state.lastEventID = eventID
			if persist {
				t.storeResumeState(state)
			}
		}
		if !dataSeen {
			eventType, eventID, eventIDSeen = "", "", false
			return nil
		}
		if data.Len() == 0 {
			eventType, eventID, eventIDSeen, dataSeen = "", "", false, false
			return nil
		}
		if eventType == "endpoint" {
			return &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("legacy endpoint event is not supported"),
			}
		}
		err := t.peer.dispatch([]byte(data.String()))
		eventType, eventID, eventIDSeen, dataSeen = "", "", false, false
		return err
	}
	firstLine := true
	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		if firstLine {
			lineBytes = bytes.TrimPrefix(lineBytes, []byte{0xef, 0xbb, 0xbf})
			firstLine = false
		}
		if !utf8.Valid(lineBytes) {
			return state, &InvalidPayloadError{
				Subject: streamableHTTPSSESubject,
				Err:     errors.New("event stream is not valid UTF-8"),
			}
		}
		line := string(lineBytes)
		if line == "" {
			if err := dispatch(); err != nil {
				return state, err
			}
			continue
		}
		field, value, hasColon := strings.Cut(line, ":")
		if !hasColon {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case dataField:
			if dataSeen {
				data.WriteByte('\n')
			}
			dataSeen = true
			data.WriteString(value)
		case "event":
			eventType = value
		case "id":
			eventID = value
			eventIDSeen = true
		case "retry":
			if delay, ok := parseSSERetry(value); ok {
				state.retryDelay = delay
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return state, ctxErr
		}
		return state, &InvalidPayloadError{Subject: streamableHTTPSSESubject, Err: err}
	}
	return state, &sseBoundaryError{state: state}
}

func splitSSELines(data []byte, atEOF bool) (int, []byte, error) {
	for index, char := range data {
		switch char {
		case '\n':
			return index + 1, data[:index], nil
		case '\r':
			if index+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			advance := index + 1
			if index+1 < len(data) && data[index+1] == '\n' {
				advance++
			}
			return advance, data[:index], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func parseSSERetry(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return 0, false
		}
	}
	milliseconds, err := strconv.ParseUint(value, 10, 64)
	if err != nil || milliseconds > uint64((1<<63-1)/int64(time.Millisecond)) {
		return 0, false
	}
	delay := time.Duration(milliseconds) * time.Millisecond
	if delay < httpPollRetryDelay {
		delay = httpPollRetryDelay
	} else if delay > httpMaxRetryDelay {
		delay = httpMaxRetryDelay
	}
	return delay, true
}

func (t *StreamableHTTPTransport) snapshotResumeState() sseResumeState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return sseResumeState{lastEventID: t.lastEventID, retryDelay: t.pollRetryDelay}
}

func (t *StreamableHTTPTransport) storeResumeState(state sseResumeState) {
	t.mu.Lock()
	t.lastEventID = state.lastEventID
	t.pollRetryDelay = state.retryDelay
	t.mu.Unlock()
}

func isInitializeRequest(body []byte) bool {
	var request struct {
		Method string `json:"method"`
	}
	return json.Unmarshal(body, &request) == nil && request.Method == MethodInitialize
}

func isJSONRPCRequest(body []byte) bool {
	_, ok := jsonRPCRequestID(body)
	return ok
}

func jsonRPCRequestID(body []byte) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil, false
	}
	id, hasID := fields["id"]
	_, hasMethod := fields["method"]
	return id, hasID && hasMethod
}

func (t *StreamableHTTPTransport) acceptSessionHeader(sessionID string, initialize bool) error {
	if sessionID == "" {
		return nil
	}
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sessionID == "" {
		if !initialize {
			return &InvalidPayloadError{
				Subject: mcpSessionIDHeader,
				Err:     errors.New("session may only be established by initialize"),
			}
		}
		t.sessionID = sessionID
		return nil
	}
	if t.sessionID != sessionID {
		return &InvalidPayloadError{
			Subject: mcpSessionIDHeader,
			Err:     errors.New("server attempted to replace active session"),
		}
	}
	return nil
}

func validateSessionID(value string) error {
	if !utf8.ValidString(value) {
		return &InvalidPayloadError{Subject: mcpSessionIDHeader, Err: errors.New("not valid UTF-8")}
	}
	for _, char := range []byte(value) {
		if char < 0x21 || char > 0x7e {
			return &InvalidPayloadError{
				Subject: mcpSessionIDHeader,
				Err:     errors.New("contains non-visible ASCII"),
			}
		}
	}
	return nil
}

func (t *StreamableHTTPTransport) hasSession() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sessionID != ""
}

func (t *StreamableHTTPTransport) MaxStreamBytes() int { return t.maxStreamBytes }

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
		started := t.started
		getStarted := t.getStarted
		cancel := t.cancel
		peer := t.peer
		sessionID := t.sessionID
		t.mu.Unlock()
		if peer != nil {
			peer.close(ErrTransportClosed)
		}
		for _, cancelPost := range cancelPosts {
			cancelPost()
		}
		t.deleteSession(sessionID)
		if cancel != nil {
			cancel()
		}
		t.postWG.Wait()
		t.resumeWG.Wait()
		if started && getStarted {
			<-t.getDone
		}
	})
	return nil
}

func (t *StreamableHTTPTransport) deleteSession(sessionID string) {
	if sessionID == "" {
		return
	}
	ctx, stop := context.WithTimeout(context.Background(), httpCloseTimeout)
	defer stop()
	request, err := t.buildRequest(ctx, http.MethodDelete, nil)
	if err != nil {
		return
	}
	// #nosec G704 -- endpoint validation and safe dialing are enforced in Start/default client.
	//nolint:bodyclose // Closed immediately through httptool helper.
	response, err := t.client.Do(request)
	if err == nil {
		httptool.CloseResponseBody(ctx, response.Body)
	}
}

var _ Transport = (*StreamableHTTPTransport)(nil)
var _ ProtocolVersionTransport = (*StreamableHTTPTransport)(nil)
var _ OperationPhaseTransport = (*StreamableHTTPTransport)(nil)
var _ StreamByteCapTransport = (*StreamableHTTPTransport)(nil)
