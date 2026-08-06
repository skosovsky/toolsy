package mcp

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
)

func validInitializeParams() InitializeParams {
	return InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo:      Implementation{Name: "test-client", Version: "1"},
	}
}

type fakePending struct {
	id           json.RawMessage
	ch           chan callResult
	deliveryDone chan struct{}
	deliveryOnce sync.Once
	sent         atomic.Bool
	mu           sync.Mutex
	done         bool
	cancelled    bool
	hooks        []func()
}

func newFakePending(id json.RawMessage) *fakePending {
	return &fakePending{
		id:           append(json.RawMessage(nil), id...),
		ch:           make(chan callResult, 1),
		deliveryDone: make(chan struct{}),
	}
}

func (p *fakePending) OnComplete(hook func()) {
	if hook == nil {
		return
	}
	p.mu.Lock()
	if !p.done {
		p.hooks = append(p.hooks, hook)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	hook()
}

func (p *fakePending) complete(result callResult) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}
	p.done = true
	hooks := append([]func(){}, p.hooks...)
	p.hooks = nil
	p.mu.Unlock()
	for _, hook := range hooks {
		hook()
	}
	p.ch <- result
	close(p.ch)
}

func (p *fakePending) ID() json.RawMessage { return append(json.RawMessage(nil), p.id...) }

func (p *fakePending) DeliveryDone() <-chan struct{} { return p.deliveryDone }
func (p *fakePending) WasSent() bool                 { return p.sent.Load() }

func (p *fakePending) markSent() {
	p.sent.Store(true)
	p.finishDelivery()
}

func (p *fakePending) finishDelivery() {
	p.deliveryOnce.Do(func() { close(p.deliveryDone) })
}

func (p *fakePending) CancelPending() bool {
	p.mu.Lock()
	if p.done {
		cancelled := p.cancelled
		p.mu.Unlock()
		return cancelled
	}
	p.done = true
	p.cancelled = true
	hooks := append([]func(){}, p.hooks...)
	p.hooks = nil
	p.mu.Unlock()
	for _, hook := range hooks {
		hook()
	}
	p.ch <- callResult{err: context.Canceled}
	close(p.ch)
	return true
}

func (p *fakePending) Await(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		if p.CancelPending() {
			return nil, ctx.Err()
		}
		result, ok := <-p.ch
		if !ok {
			return nil, ErrTransportClosed
		}
		return result.result, result.err
	case result, ok := <-p.ch:
		if !ok {
			return nil, ErrTransportClosed
		}
		return result.result, result.err
	}
}

type capturedRequest struct {
	Method string
	Params json.RawMessage
	ID     json.RawMessage
}

type capturedNotification struct {
	Method string
	Params json.RawMessage
}

type fakeTransport struct {
	mu            sync.Mutex
	started       bool
	closed        bool
	version       string
	nextID        atomic.Uint64
	closeCalls    atomic.Uint64
	requests      []capturedRequest
	notifications []capturedNotification
	pending       map[string]*fakePending
	requestHook   func(capturedRequest) (json.RawMessage, error, bool)
	notifyHook    func(capturedNotification)
	handler       RequestHandler
	notify        map[string]NotificationHandler
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		pending: make(map[string]*fakePending),
		notify:  make(map[string]NotificationHandler),
	}
}

func (t *fakeTransport) Start(context.Context) error {
	t.mu.Lock()
	t.started = true
	t.mu.Unlock()
	return nil
}

func (t *fakeTransport) Request(
	_ context.Context,
	method string,
	params any,
) (PendingRequest, error) {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	id := json.RawMessage(strconv.FormatUint(t.nextID.Add(1), 10))
	captured := capturedRequest{Method: method, Params: paramsRaw, ID: id}
	pending := newFakePending(id)
	pending.markSent()
	t.mu.Lock()
	t.requests = append(t.requests, captured)
	t.pending[string(id)] = pending
	hook := t.requestHook
	t.mu.Unlock()
	if hook != nil {
		result, hookErr, complete := hook(captured)
		if complete {
			pending.complete(callResult{result: result, err: hookErr})
		}
	}
	return pending, nil
}

func (t *fakeTransport) Notify(_ context.Context, method string, params any) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	t.mu.Lock()
	notification := capturedNotification{Method: method, Params: paramsRaw}
	t.notifications = append(t.notifications, notification)
	hook := t.notifyHook
	t.mu.Unlock()
	if hook != nil {
		hook(notification)
	}
	return nil
}

func (t *fakeTransport) OnRequest(handler RequestHandler) { t.handler = handler }

func (t *fakeTransport) OnNotification(method string, handler NotificationHandler) {
	t.notify[method] = handler
}

func (t *fakeTransport) Close() error {
	t.closeCalls.Add(1)
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	return nil
}

func (t *fakeTransport) SetProtocolVersion(version string) { t.version = version }

func (t *fakeTransport) emit(method string, params any) {
	raw, _ := json.Marshal(params)
	if handler := t.notify[method]; handler != nil {
		handler(raw)
	}
}

func (t *fakeTransport) complete(id string, result any, err error) {
	raw, _ := json.Marshal(result)
	t.mu.Lock()
	pending := t.pending[id]
	t.mu.Unlock()
	pending.complete(callResult{result: raw, err: err})
}

func initializeResult(capabilities ServerCapabilities) json.RawMessage {
	raw, _ := json.Marshal(InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    capabilities,
		ServerInfo:      Implementation{Name: "test-server", Version: "1.0.0"},
	})
	return raw
}

var _ Transport = (*fakeTransport)(nil)
var _ ProtocolVersionTransport = (*fakeTransport)(nil)
var _ CompletionPendingRequest = (*fakePending)(nil)
var _ DeliveryPendingRequest = (*fakePending)(nil)
var _ CancellablePendingRequest = (*fakePending)(nil)
