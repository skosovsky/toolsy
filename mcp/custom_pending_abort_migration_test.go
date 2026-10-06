package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

// Expose only the required prepared/completion interfaces. In particular, the
// client cannot rely on the underlying peer's optional local cancellation port.
type customAbortPending struct {
	prepared     PreparedRequest
	completion   CompletionPendingRequest
	awaitDone    chan struct{}
	completed    chan struct{}
	once         sync.Once
	completeOnce sync.Once
}

func (p *customAbortPending) ID() json.RawMessage   { return p.prepared.ID() }
func (p *customAbortPending) Deliver() error        { return p.prepared.Deliver() }
func (p *customAbortPending) Abort(err error) error { return p.prepared.Abort(err) }
func (p *customAbortPending) OnComplete(hook func()) {
	p.completion.OnComplete(func() { hook(); p.completeOnce.Do(func() { close(p.completed) }) })
}
func (p *customAbortPending) Await(ctx context.Context) (json.RawMessage, error) {
	defer p.once.Do(func() { close(p.awaitDone) })
	return p.prepared.Await(ctx)
}

type customAbortTransport struct {
	*fakeTransport

	first *customAbortPending
}

func (t *customAbortTransport) PrepareRequest(ctx context.Context, method string, params any) (PreparedRequest, error) {
	pending, err := t.fakeTransport.PrepareRequest(ctx, method, params)
	if err != nil || method != MethodToolsCall {
		return pending, err
	}
	wrapped := &customAbortPending{
		prepared:   pending,
		completion: pending.(CompletionPendingRequest),
		awaitDone:  make(chan struct{}),
		completed:  make(chan struct{}),
	}
	if t.first == nil {
		t.first = wrapped
	}
	return wrapped, nil
}

func TestCustomPendingConsumerAbortCleansInvocationWithoutClosingTransport(t *testing.T) {
	// Arrange. First call never receives a remote terminal; Await honors its ctx.
	transport := &customAbortTransport{fakeTransport: newFakeTransport()}
	var token ProgressToken
	calls := 0
	migrationToolScript(t, transport.fakeTransport, nil)
	ordinaryHook := transport.requestHook
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method != MethodToolsCall {
			return ordinaryHook(request)
		}
		calls++
		if calls > 1 {
			return ordinaryHook(request)
		}
		var params ToolsCallParams
		require.NoError(t, json.Unmarshal(request.Params, &params))
		token = params.Meta.ProgressToken
		require.NoError(t, transport.emit(MethodProgress, ProgressParams{ProgressToken: token, Progress: 1}))
		return nil, nil, false
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	proxy := migrationProgressProxy(context.Background(), t, client)
	cause := errors.New("consumer stopped")
	yields := 0

	// Act. The background parent is deliberately never cancelled by the host.
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { yields++; return cause },
	)

	// Assert. Invocation cleanup must not require global transport shutdown.
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, yields)
	require.NotNil(t, transport.first)
	_, cancellable := any(transport.first).(CancellablePendingRequest)
	_, delivery := any(transport.first).(DeliveryPendingRequest)
	_, ownership := any(transport.first).(CancellationNotificationPending)
	require.False(t, cancellable)
	require.False(t, delivery)
	require.False(t, ownership)
	select {
	case <-transport.first.awaitDone:
	case <-time.After(time.Second):
		t.Fatal("consumer abort leaked custom Await with background parent")
	}
	select {
	case <-transport.first.completed:
	default:
		t.Fatal("custom completion hooks were not run before Await cleanup")
	}
	active := false
	client.progressCallbacks.Range(func(any, any) bool { active = true; return false })
	require.False(t, active)
	require.NoError(t, transport.emit(MethodProgress, ProgressParams{ProgressToken: token, Progress: 2}))
	require.Equal(t, 1, yields, "late progress must not reach stopped consumer")
	require.Zero(t, transport.closeCalls.Load())
	transport.mu.Lock()
	notifications := append([]capturedNotification(nil), transport.notifications...)
	transport.mu.Unlock()
	require.LessOrEqual(t, len(notifications), 1)
	for _, notification := range notifications {
		require.Equal(t, MethodCancelled, notification.Method)
		var params CancelledParams
		require.NoError(t, json.Unmarshal(notification.Params, &params))
		require.JSONEq(t, string(transport.first.ID()), string(params.RequestID))
	}
	results := 0
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(chunk toolsy.Chunk) error {
			require.Equal(t, toolsy.EventResult, chunk.Event)
			results++
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, results)
	require.Zero(t, transport.closeCalls.Load())
}
