package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

// quietConnectLogger returns a [ClientOption] that discards client logs during Connect tests.
func quietConnectLogger() ClientOption {
	return WithClientLogger(slog.New(slog.DiscardHandler))
}

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

type streamCapCaptureTransport struct {
	connectCaptureTransport

	streamCap int
}

func (t *streamCapCaptureTransport) MaxStreamBytes() int {
	return t.streamCap
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

	client, err := Connect(context.Background(), transport,
		quietConnectLogger(),
		WithClientRoots([]string{"/workspace"}),
	)
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

	client, err := Connect(context.Background(), transport, quietConnectLogger())
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_InitializeCallFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callErr: errors.New("initialize call failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport, quietConnectLogger())
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

	client, err := Connect(context.Background(), transport, quietConnectLogger())
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

	client, err := Connect(context.Background(), transport, quietConnectLogger())
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "notifications/initialized")
	require.Equal(t, MethodInitialized, base.notifiedMethod)
	require.Equal(t, 1, base.closeCalls)
}

func TestHandleToolCallResult_ErrorChunkUsesStructuredWire(t *testing.T) {
	client := &Client{}
	rawResult, err := json.Marshal(ToolsCallResult{
		Content: []ContentItem{{Type: "text", Text: "permission denied"}},
		IsError: true,
	})
	require.NoError(t, err)

	var got toolsy.Chunk
	err = client.handleToolCallResult(
		context.Background(),
		callResultWithErr{res: rawResult, requestID: "req-1"},
		func(c toolsy.Chunk) error {
			got = c
			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, got.IsError)
	require.Equal(t, toolsy.MimeTypeToolErrorJSON, got.MimeType)

	var wire struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(got.Data, &wire))
	require.Equal(t, string(toolsy.CodeValidationFailed), wire.Code)
}

func TestHandleToolCallResult_ReadLimitExceeded_MapsValidation(t *testing.T) {
	client := &Client{}
	err := client.handleToolCallResult(
		context.Background(),
		callResultWithErr{err: textprocessor.ErrReadLimitExceeded, requestID: "req-limit"},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestGetResourceTool_ReadLimitExceeded(t *testing.T) {
	client := &Client{transport: &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}}
	tool, err := client.GetResourceTool()
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"uri":"file:///x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestGetPrompt_ReadLimitExceeded(t *testing.T) {
	client := &Client{transport: &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}}
	_, err := client.GetPrompt(context.Background(), "greeting", nil)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestGetResourceTool_ReadLimit_CustomStreamCap(t *testing.T) {
	const customCap = 2048
	client := &Client{transport: &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded},
		streamCap:               customCap,
	}}
	tool, err := client.GetResourceTool()
	require.NoError(t, err)

	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"uri":"file:///x"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
	require.NotContains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestHandleToolCallResult_ReadLimit_CustomStreamCap(t *testing.T) {
	const customCap = 2048
	client := &Client{transport: &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{},
		streamCap:               customCap,
	}}
	err := client.handleToolCallResult(
		context.Background(),
		callResultWithErr{err: textprocessor.ErrReadLimitExceeded, requestID: "req-limit"},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
}

func TestGetTools_MapsAnnotationsToManifest(t *testing.T) {
	toolsList := ToolsListResult{
		Tools: []MCPTool{
			{
				Name:        "read_tool",
				Description: "read only",
				InputSchema: []byte(`{"type":"object"}`),
				Annotations: &ToolAnnotations{ReadOnlyHint: new(true)},
			},
			{
				Name:        "delete_tool",
				Description: "destructive",
				InputSchema: []byte(`{"type":"object"}`),
				Annotations: &ToolAnnotations{
					DestructiveHint: new(true),
					IdempotentHint:  new(true),
				},
			},
		},
	}
	resultBytes, err := json.Marshal(toolsList)
	require.NoError(t, err)

	client := &Client{transport: &connectCaptureTransport{callResult: resultBytes}}
	ctx := context.Background()

	var tools []toolsy.Tool
	for tool, iterErr := range client.GetTools(ctx) {
		require.NoError(t, iterErr)
		tools = append(tools, tool)
	}
	require.Len(t, tools, 2)

	byName := make(map[string]toolsy.Tool, len(tools))
	for _, tool := range tools {
		byName[tool.Manifest().Name] = tool
	}

	readTool := byName["read_tool"]
	require.NotNil(t, readTool)
	require.True(t, readTool.Manifest().ReadOnly)
	require.False(t, readTool.Manifest().Dangerous)

	deleteTool := byName["delete_tool"]
	require.NotNil(t, deleteTool)
	require.True(t, deleteTool.Manifest().Dangerous)
	require.True(t, deleteTool.Manifest().Idempotent)
	require.False(t, deleteTool.Manifest().ReadOnly)
}

func TestConnect_Initialize_ReadLimitMapsValidation(t *testing.T) {
	t.Parallel()
	base := &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}
	_, err := Connect(context.Background(), base, quietConnectLogger())
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "byte limit")
	require.Contains(t, te.Reason, "MCP initialize response")
	require.Equal(t, MethodInitialize, base.calledMethod)
}

func TestGetTools_ReadLimitExceeded(t *testing.T) {
	t.Parallel()
	client := &Client{transport: &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}}
	var iterErr error
	for _, err := range client.GetTools(context.Background()) {
		if err != nil {
			iterErr = err
			break
		}
	}
	require.Error(t, iterErr)
	te, ok := toolsy.AsToolError(iterErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "byte limit")
	require.Contains(t, te.Reason, "MCP tools list response")
}

func TestGetPrompts_ReadLimitExceeded(t *testing.T) {
	t.Parallel()
	client := &Client{transport: &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}}
	var iterErr error
	for _, err := range client.GetPrompts(context.Background()) {
		if err != nil {
			iterErr = err
			break
		}
	}
	require.Error(t, iterErr)
	te, ok := toolsy.AsToolError(iterErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "byte limit")
}

func TestConnect_Initialize_ReadLimit_CustomStreamCap(t *testing.T) {
	t.Parallel()
	const customCap = 2048
	base := &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded},
		streamCap:               customCap,
	}
	_, err := Connect(context.Background(), base, quietConnectLogger())
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
}

func TestGetTools_ReadLimit_CustomStreamCap(t *testing.T) {
	t.Parallel()
	const customCap = 2048
	client := &Client{transport: &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded},
		streamCap:               customCap,
	}}
	var iterErr error
	for _, err := range client.GetTools(context.Background()) {
		if err != nil {
			iterErr = err
			break
		}
	}
	require.Error(t, iterErr)
	te, ok := toolsy.AsToolError(iterErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
}

func TestGetPrompts_ReadLimit_CustomStreamCap(t *testing.T) {
	t.Parallel()
	const customCap = 2048
	client := &Client{transport: &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded},
		streamCap:               customCap,
	}}
	var iterErr error
	for _, err := range client.GetPrompts(context.Background()) {
		if err != nil {
			iterErr = err
			break
		}
	}
	require.Error(t, iterErr)
	te, ok := toolsy.AsToolError(iterErr)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
}

func TestMapCallReadLimit_CancelOverLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &Client{}
	err := client.mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "MCP tools list response")
	require.ErrorIs(t, err, context.Canceled)
}

func TestMapCallReadLimit_DeadlineOverLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	client := &Client{}
	err := client.mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "MCP tools list response")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMapCallReadLimit_TimeoutOverLimit(t *testing.T) {
	t.Parallel()
	client := &Client{}
	err := client.mapCallReadLimitFor(
		context.Background(),
		fmt.Errorf("slow: %w", toolsy.ErrTimeout),
		"MCP tools list response",
	)
	require.ErrorIs(t, err, toolsy.ErrTimeout)
}

func TestHandleToolCallResult_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &Client{}
	err := client.handleToolCallResult(
		ctx,
		callResultWithErr{err: textprocessor.ErrReadLimitExceeded},
		func(toolsy.Chunk) error { return nil },
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestMapCallReadLimitFor_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	client := &Client{}
	composite := fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := client.mapCallReadLimitFor(context.Background(), composite, "MCP tools list response")
	require.ErrorIs(t, err, context.Canceled)
}

func TestHandleToolCallResult_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	client := &Client{}
	composite := fmt.Errorf(
		"stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	err := client.handleToolCallResult(
		context.Background(),
		callResultWithErr{err: composite},
		func(toolsy.Chunk) error { return nil },
	)
	require.ErrorIs(t, err, context.Canceled)
}
