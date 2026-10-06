package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type capturedRequest struct {
	Method string
	Params json.RawMessage
	ID     json.RawMessage
}

type capturedNotification struct {
	Method string
	Params json.RawMessage
}

// fakeTransport uses the real prepared/pending lifecycle. Preparation is inert;
// hooks and captured wire requests run only after Deliver, with real completion,
// cancellation, delivery and single-notification ownership.
type fakeTransport struct {
	mu            sync.Mutex
	started       bool
	closed        bool
	peer          *rpcPeer
	closeCalls    atomic.Uint64
	preparations  []capturedRequest
	requests      []capturedRequest
	peerWrites    [][]byte
	notifications []capturedNotification
	pending       map[string]*pendingRequest
	requestHook   func(capturedRequest) (json.RawMessage, error, bool)
	notifyHook    func(capturedNotification)
	notify        map[string]NotificationHandler
}

func newFakeTransport() *fakeTransport {
	transport := &fakeTransport{
		pending: make(map[string]*pendingRequest),
		notify:  make(map[string]NotificationHandler),
	}
	transport.peer = newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		transport.mu.Lock()
		transport.peerWrites = append(transport.peerWrites, bytes.Clone(body))
		transport.mu.Unlock()
		return nil
	})
	return transport
}

func (t *fakeTransport) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransportClosed
	}
	t.started = true
	return nil
}

func cloneCapturedRequest(request capturedRequest) capturedRequest {
	return capturedRequest{Method: request.Method, Params: bytes.Clone(request.Params), ID: bytes.Clone(request.ID)}
}

func (t *fakeTransport) PrepareRequest(ctx context.Context, method string, params any) (PreparedRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateOutgoingRequest(method, params); err != nil {
		return nil, err
	}
	t.mu.Lock()
	started := t.started && !t.closed
	t.mu.Unlock()
	if !started {
		return nil, ErrTransportClosed
	}
	pending, body, err := t.peer.beginRequest(method, params)
	if err != nil {
		return nil, err
	}
	var request Request
	if err := json.Unmarshal(body, &request); err != nil {
		t.peer.failPending(pending, err)
		return nil, err
	}
	captured := capturedRequest{Method: method, Params: bytes.Clone(request.Params), ID: pending.ID()}
	t.mu.Lock()
	t.preparations = append(t.preparations, cloneCapturedRequest(captured))
	t.pending[string(captured.ID)] = pending
	t.mu.Unlock()
	return &preparedRequest{
		pendingRequest: pending,
		ctx:            ctx,
		deliver: func() {
			defer pending.finishDelivery()
			t.mu.Lock()
			closed := t.closed
			hook := t.requestHook
			if !closed {
				t.requests = append(t.requests, cloneCapturedRequest(captured))
			}
			t.mu.Unlock()
			if closed {
				t.peer.failPending(pending, ErrTransportClosed)
				return
			}
			pending.markSent()
			if hook != nil {
				result, hookErr, complete := hook(cloneCapturedRequest(captured))
				if complete {
					t.finish(pending, result, hookErr)
				}
			}
		},
	}, nil
}

// finish allows malformed domain result fixtures, just like a wire JSON-RPC
// result. Ownership and pending cleanup still follow the real peer terminal path.
func (t *fakeTransport) finish(pending *pendingRequest, result json.RawMessage, err error) {
	t.peer.pendingMu.Lock()
	active := t.peer.pending[pending.key] == pending
	if active {
		delete(t.peer.pending, pending.key)
	}
	t.peer.pendingMu.Unlock()
	if active {
		pending.complete(callResult{result: bytes.Clone(result), err: err})
	}
}

func (t *fakeTransport) Notify(ctx context.Context, method string, params any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateOutgoingNotification(method, params); err != nil {
		return err
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrTransportClosed
	}
	notification := capturedNotification{Method: method, Params: bytes.Clone(paramsRaw)}
	t.notifications = append(t.notifications, notification)
	hook := t.notifyHook
	t.mu.Unlock()
	if hook != nil {
		hook(capturedNotification{Method: method, Params: bytes.Clone(paramsRaw)})
	}
	return nil
}

func (t *fakeTransport) OnNotification(method string, handler NotificationHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if handler == nil {
		delete(t.notify, method)
	} else {
		t.notify[method] = handler
	}
}

func (t *fakeTransport) Close() error {
	t.closeCalls.Add(1)
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.peer.close(ErrTransportClosed)
	return nil
}

func (t *fakeTransport) emit(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	t.mu.Lock()
	handler := t.notify[method]
	closed := t.closed
	t.mu.Unlock()
	if !closed && handler != nil {
		handler(bytes.Clone(raw))
	}
	return nil
}

func (t *fakeTransport) complete(id string, result any, err error) error {
	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return marshalErr
	}
	t.mu.Lock()
	pending := t.pending[id]
	t.mu.Unlock()
	if pending == nil {
		return fmt.Errorf("unknown test pending ID %s", id)
	}
	t.finish(pending, raw, err)
	return nil
}

var _ Transport = (*fakeTransport)(nil)
var _ CompletionPendingRequest = (*preparedRequest)(nil)
var _ DeliveryPendingRequest = (*preparedRequest)(nil)
var _ CancellablePendingRequest = (*preparedRequest)(nil)
var _ CancellationNotificationPending = (*preparedRequest)(nil)
