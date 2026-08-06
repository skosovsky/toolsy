package mcp

import (
	"context"
	"encoding/json"
)

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

type RequestHandler func(ctx context.Context, request Request) (json.RawMessage, *JSONRPCError)

type NotificationHandler func(params json.RawMessage)

// Transport is a bidirectional JSON-RPC peer transport.
type Transport interface {
	Start(ctx context.Context) error
	Request(ctx context.Context, method string, params any) (PendingRequest, error)
	Notify(ctx context.Context, method string, params any) error
	OnRequest(handler RequestHandler)
	OnNotification(method string, handler NotificationHandler)
	Close() error
}

type ProtocolVersionTransport interface {
	SetProtocolVersion(version string)
}

type OperationPhaseTransport interface {
	Activate()
}

type StreamByteCapTransport interface {
	MaxStreamBytes() int
}
