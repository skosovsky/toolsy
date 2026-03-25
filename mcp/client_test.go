package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type notifyCaptureTransport struct {
	notifyCtx context.Context
	notifyErr error
	method    string
	params    any
}

func (t *notifyCaptureTransport) Start(context.Context) error { return nil }

func (t *notifyCaptureTransport) Call(context.Context, string, any) ([]byte, string, error) {
	return nil, "", nil
}

func (t *notifyCaptureTransport) Notify(ctx context.Context, method string, params any) error {
	t.notifyCtx = ctx
	t.notifyErr = ctx.Err()
	t.method = method
	t.params = params
	return nil
}

func (t *notifyCaptureTransport) OnNotification(string, func([]byte)) {}

func (t *notifyCaptureTransport) Close() error { return nil }

func TestNotifyCancelledRequestUsesBoundedContext(t *testing.T) {
	transport := &notifyCaptureTransport{}
	client := NewClient(transport)
	type traceKey struct{}
	parentCtx := context.WithValue(context.Background(), traceKey{}, "trace-123")
	parentCtx, cancelParent := context.WithCancel(parentCtx)
	cancelParent()

	before := time.Now()
	client.notifyCancelledRequest(parentCtx, "req-1")
	after := time.Now()

	require.NotNil(t, transport.notifyCtx)
	require.Equal(t, MethodCancelled, transport.method)
	require.Equal(t, "trace-123", transport.notifyCtx.Value(traceKey{}))
	require.NoError(t, transport.notifyErr, "cancel notify context must be detached from parent cancellation")
	deadline, ok := transport.notifyCtx.Deadline()
	require.True(t, ok, "expected bounded cancel notification context")
	require.True(t, deadline.After(before))
	require.True(t, deadline.Before(after.Add(6*time.Second)))
}
