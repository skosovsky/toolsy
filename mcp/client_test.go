package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustInitializeResultJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ServerCapabilities{},
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	})
	require.NoError(t, err)
	return data
}

type connectCaptureTransport struct {
	startErr  error
	callErr   error
	notifyErr error

	callResult []byte
	requestID  string

	startCalls int
	closeCalls int

	calledMethod string
	calledParams any

	notifiedMethod string
	notifiedParams any

	onNotificationMethod string
}

func (t *connectCaptureTransport) Start(context.Context) error {
	t.startCalls++
	return t.startErr
}

func (t *connectCaptureTransport) Call(
	_ context.Context,
	method string,
	params any,
) ([]byte, string, error) {
	t.calledMethod = method
	t.calledParams = params
	if t.callErr != nil {
		return nil, "", t.callErr
	}
	return t.callResult, t.requestID, nil
}

func (t *connectCaptureTransport) Notify(_ context.Context, method string, params any) error {
	t.notifiedMethod = method
	t.notifiedParams = params
	return t.notifyErr
}

func (t *connectCaptureTransport) OnNotification(method string, _ func([]byte)) {
	t.onNotificationMethod = method
}

func (t *connectCaptureTransport) Close() error {
	t.closeCalls++
	return nil
}

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
	client := &Client{transport: transport}
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

func TestConnect_EagerHandshakeSuccess(t *testing.T) {
	base := &connectCaptureTransport{
		callResult: mustInitializeResultJSON(t),
		requestID:  "req-init",
	}
	transport := base

	client, err := Connect(context.Background(), transport, WithClientRoots([]string{"/workspace"}))
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, base.startCalls)
	require.Equal(t, MethodProgress, base.onNotificationMethod)
	require.Equal(t, MethodInitialize, base.calledMethod)
	require.Equal(t, MethodInitialized, base.notifiedMethod)
	require.NotNil(t, client.serverCaps)

	paramsBytes, err := json.Marshal(base.calledParams)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal(paramsBytes, &params))
	require.Equal(t, "2024-11-05", params["protocolVersion"])
	require.Equal(t, []any{"/workspace"}, params["roots"])

	require.NoError(t, client.Close())
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_StartFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		startErr: errors.New("start failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport)
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_InitializeCallFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callErr: errors.New("initialize call failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport)
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, MethodInitialize, base.calledMethod)
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_InitializeParseFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callResult: []byte(`{"invalid_json"`),
	}
	transport := base

	client, err := Connect(context.Background(), transport)
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "parse initialize result")
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_InitializedNotifyFailureReturnsErrorAndClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callResult: mustInitializeResultJSON(t),
		notifyErr:  errors.New("notify failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport)
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "notifications/initialized")
	require.Equal(t, MethodInitialized, base.notifiedMethod)
	require.Equal(t, 1, base.closeCalls)
}
