package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
)

// defaultTransportMaxFrameBytes is the default per-message frame budget.
const defaultTransportMaxFrameBytes = 1024 * 1024

func contextUntilPendingTerminal(
	parent context.Context,
	terminal <-chan struct{},
) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-terminal:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		cancel()
		<-watcherDone
	}
}

type PendingRequest interface {
	ID() json.RawMessage
	Await(ctx context.Context) (json.RawMessage, error)
}

// PreparedRequest exposes a request ID before any bytes reach the wire. The
// caller registers all ID-correlated state and then calls Deliver exactly once.
type PreparedRequest interface {
	PendingRequest
	Deliver() error
	Abort(cause error) error
}

var errRequestAlreadyDelivered = errors.New("mcp: prepared request already delivered")
var errPreparedRequestFinalized = errors.New("mcp: prepared request already delivered or aborted")

const (
	preparedRequestReady uint32 = iota
	preparedRequestDelivered
	preparedRequestAborted
)

type preparedRequest struct {
	*pendingRequest

	ctx     context.Context
	deliver func()
	state   atomic.Uint32
}

func (p *preparedRequest) Deliver() error {
	if !p.state.CompareAndSwap(preparedRequestReady, preparedRequestDelivered) {
		return errRequestAlreadyDelivered
	}
	if err := p.ctx.Err(); err != nil {
		p.finishDelivery()
		p.peer.failPending(p.pendingRequest, err)
		return err
	}
	select {
	case <-p.terminal:
		p.finishDelivery()
		return context.Canceled
	default:
	}
	p.deliver()
	return nil
}

func (p *preparedRequest) Abort(cause error) error {
	if !p.state.CompareAndSwap(preparedRequestReady, preparedRequestAborted) {
		return errPreparedRequestFinalized
	}
	if cause == nil {
		cause = context.Canceled
	}
	p.finishDelivery()
	p.peer.failPending(p.pendingRequest, cause)
	return nil
}

// CompletionPendingRequest exposes the wire terminal boundary. Callbacks run
// before Await is released and are used to retire progress routes losslessly.
type CompletionPendingRequest interface {
	PendingRequest
	OnComplete(func())
}

// DeliveryPendingRequest exposes whether the original request reached the wire.
// DeliveryDone closes after either successful delivery or a terminal send failure.
type DeliveryPendingRequest interface {
	PendingRequest
	DeliveryDone() <-chan struct{}
	WasSent() bool
}

// CancellablePendingRequest atomically claims cancellation while the request is active.
type CancellablePendingRequest interface {
	PendingRequest
	CancelPending() bool
}

// CancellationNotificationPending claims the single stdio cancellation
// notification independently from idempotent local pending cancellation.
type CancellationNotificationPending interface {
	PendingRequest
	ClaimCancellationNotification() bool
}

type NotificationHandler func(params json.RawMessage)

// RequestScopedNotificationHandler receives a notification only after the
// transport has proven that it belongs to the originating request stream.
type RequestScopedNotificationHandler func(requestID json.RawMessage, params json.RawMessage)

// RequestScopedNotificationTransport exposes transport-owned correlation for
// request streams whose wire notifications do not carry a correlation token.
type RequestScopedNotificationTransport interface {
	OnRequestNotification(method string, handler RequestScopedNotificationHandler)
}

// Transport sends client requests and receives server responses/notifications.
// MCP 2026-07-28 does not permit server-initiated JSON-RPC requests.
type Transport interface {
	Start(ctx context.Context) error
	PrepareRequest(ctx context.Context, method string, params any) (PreparedRequest, error)
	Notify(ctx context.Context, method string, params any) error
	OnNotification(method string, handler NotificationHandler)
	Close() error
}

// HTTPToolHeaderBinding describes one validated x-mcp-header annotation.
type HTTPToolHeaderBinding struct {
	Header string
	Path   []string
	Type   string
}

// ToolHeaderTransport accepts tool header descriptors discovered by the client.
// ReplaceToolHeaderBindings atomically replaces all bindings or returns an error
// without changing them. It must finish in bounded time and must not reenter
// Client methods: the client holds its authority lock during replacement.
type ToolHeaderTransport interface {
	ReplaceToolHeaderBindings(bindings map[string][]HTTPToolHeaderBinding) error
}

type FrameByteCapTransport interface {
	MaxFrameBytes() int
}

const (
	legacyNotificationInitialized = "notifications/initialized"
	subjectMCPRequestMeta         = "MCP request _meta"
)

func validateOutgoingRequest(method string, params any) error {
	if method == "" || strings.HasPrefix(method, "notifications/") {
		return &InvalidPayloadError{
			Subject: "MCP request method",
			Err:     fmt.Errorf("method %q is not a request method", method),
		}
	}
	if isForbiddenLegacyMethod(method) {
		return &InvalidPayloadError{
			Subject: "MCP request method",
			Err:     fmt.Errorf("legacy method %q is forbidden", method),
		}
	}
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	validationErr := validateJSONValue(raw)
	if validationErr != nil {
		return &InvalidPayloadError{Subject: "MCP request params", Err: validationErr}
	}
	fields, err := decodeObjectFields(raw)
	if err != nil {
		return &InvalidPayloadError{Subject: "MCP request params", Err: err}
	}
	metaRaw, exists := fields["_meta"]
	if !exists || bytes.Equal(bytes.TrimSpace(metaRaw), []byte(jsonNull)) {
		return &InvalidPayloadError{
			Subject: subjectMCPRequestMeta,
			Err:     errors.New("required field is missing or null"),
		}
	}
	var meta RequestMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return &InvalidPayloadError{Subject: subjectMCPRequestMeta, Err: err}
	}
	if len(meta.ClientCapabilities.Extra) != 0 {
		return &InvalidPayloadError{
			Subject: "MCP request client capabilities",
			Err:     errors.New("unknown top-level capabilities must be declared under extensions"),
		}
	}
	if meta.ClientCapabilities.Roots != nil || meta.ClientCapabilities.Sampling != nil ||
		meta.ClientCapabilities.Elicitation != nil {
		return &InvalidPayloadError{
			Subject: "MCP request client capabilities",
			Err:     errors.New("deprecated client capabilities have no runtime handlers"),
		}
	}
	for key := range meta.Extra {
		if isMCPReservedMetaKey(key) {
			return &InvalidPayloadError{
				Subject: subjectMCPRequestMeta,
				Err:     fmt.Errorf("unknown reserved metadata key %q", key),
			}
		}
	}
	return nil
}

func validateOutgoingNotification(method string, params any) error {
	if method == "" || !strings.HasPrefix(method, "notifications/") {
		return &InvalidPayloadError{
			Subject: "MCP notification method",
			Err:     fmt.Errorf("method %q is not a notification method", method),
		}
	}
	if isForbiddenLegacyMethod(method) {
		return &InvalidPayloadError{
			Subject: "MCP notification method",
			Err:     fmt.Errorf("legacy method %q is forbidden", method),
		}
	}
	raw, err := marshalParams(params)
	if err != nil {
		return err
	}
	if err := validateJSONValue(raw); err != nil {
		return &InvalidPayloadError{Subject: "MCP notification params", Err: err}
	}
	if _, err := decodeObjectFields(raw); err != nil {
		return &InvalidPayloadError{Subject: "MCP notification params", Err: err}
	}
	return nil
}

func isForbiddenLegacyMethod(method string) bool {
	switch method {
	case "initialize",
		legacyNotificationInitialized,
		"roots/list",
		"notifications/roots/list_changed",
		"logging/setLevel",
		"resources/subscribe",
		"resources/unsubscribe",
		"ping":
		return true
	default:
		return false
	}
}

func isMCPReservedMetaKey(key string) bool {
	prefix, _, hasPrefix := strings.Cut(key, "/")
	if !hasPrefix {
		return false
	}
	labels := strings.Split(prefix, ".")
	return len(labels) > 1 && (labels[1] == "modelcontextprotocol" || labels[1] == "mcp")
}
