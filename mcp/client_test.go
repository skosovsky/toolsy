package mcp

import (
	"bytes"
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

func mustDiscoveryResultJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(
		DiscoverResult{
			ResultType:        ResultTypeComplete,
			SupportedVersions: []string{ProtocolVersion},
			Capabilities:      ServerCapabilities{},
			TTLMS:             JSONNumber("0"), CacheScope: CacheScopePrivate,
			Meta: ResultMeta{ServerInfo: &Implementation{Name: "test-server", Version: "1.0.0"}},
		},
	)
	require.NoError(t, err)
	return data
}

type connectCaptureTransport struct {
	inner                        *fakeTransport
	startErr, callErr, notifyErr error
	callResult                   []byte
	startCalls, closeCalls       int
	calledMethod                 string
	calledParams                 json.RawMessage
	notifiedMethod               string
	notifiedParams               any
	notificationMethods          []string
}

func (t *connectCaptureTransport) ensureInner() {
	if t.inner != nil {
		return
	}
	t.inner = newFakeTransport()
	t.inner.requestHook = func(capturedRequest) (json.RawMessage, error, bool) { return t.callResult, t.callErr, true }
}
func (t *connectCaptureTransport) Start(ctx context.Context) error {
	t.startCalls++
	t.ensureInner()
	if t.startErr != nil {
		return t.startErr
	}
	return t.inner.Start(ctx)
}

func (t *connectCaptureTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	t.calledMethod = method
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	t.calledParams = bytes.Clone(raw)
	t.ensureInner()
	return t.inner.PrepareRequest(ctx, method, params)
}
func (t *connectCaptureTransport) Notify(ctx context.Context, method string, params any) error {
	t.notifiedMethod, t.notifiedParams = method, params
	if t.notifyErr != nil {
		return t.notifyErr
	}
	t.ensureInner()
	return t.inner.Notify(ctx, method, params)
}
func (t *connectCaptureTransport) OnNotification(method string, handler NotificationHandler) {
	t.notificationMethods = append(t.notificationMethods, method)
	t.ensureInner()
	t.inner.OnNotification(method, handler)
}
func (t *connectCaptureTransport) Close() error {
	t.closeCalls++
	t.ensureInner()
	return t.inner.Close()
}

type streamCapCaptureTransport struct {
	connectCaptureTransport

	streamCap int
}

func (t *streamCapCaptureTransport) MaxStreamBytes() int { return t.streamCap }

type notifyCaptureTransport struct {
	notifyCtx context.Context
	notifyErr error
	method    string
	params    any
}

func (*notifyCaptureTransport) Start(context.Context) error { return nil }
func (*notifyCaptureTransport) PrepareRequest(context.Context, string, any) (PreparedRequest, error) {
	return nil, errors.New("notification-only fixture cannot prepare requests")
}
func (t *notifyCaptureTransport) Notify(ctx context.Context, method string, params any) error {
	t.notifyCtx, t.notifyErr, t.method, t.params = ctx, ctx.Err(), method, params
	return nil
}
func (*notifyCaptureTransport) OnNotification(string, NotificationHandler) {}
func (*notifyCaptureTransport) Close() error                               { return nil }

// Arrange explicit discovery state; the transport uses real preparation and completion.
func newReadyCaptureClient(t *testing.T, transport Transport) *Client {
	t.Helper()
	require.NoError(t, transport.Start(context.Background()))
	client := &Client{
		transport: transport,
		ready:     true,
		logger:    slog.New(slog.DiscardHandler),
		opts:      ClientOptions{ClientInfo: Implementation{Name: "boundary-test", Version: "test"}},
		server: DiscoverResult{
			Capabilities: ServerCapabilities{
				Tools:     &ToolsCapability{},
				Resources: &ResourcesCapability{},
				Prompts:   &PromptsCapability{},
			},
		},
		toolBindings:  make(map[string]struct{}),
		toolSchemas:   make(map[string]schemaValidator),
		subscriptions: make(map[string]*subscriptionState),
		invalidations: make(chan Invalidation, 1),
	}
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestNotifyCancelledRequestUsesBoundedContext(t *testing.T) {
	transport := &notifyCaptureTransport{}
	client := newReadyCaptureClient(t, transport)
	type traceKey struct{}
	parentCtx := context.WithValue(context.Background(), traceKey{}, "trace-123")
	parentCtx, cancelParent := context.WithCancel(parentCtx)
	cancelParent()

	before := time.Now()
	pending := newContractPending(1, nil, nil)
	pending.id = json.RawMessage(`"req-1"`)
	client.notifyCancelledPending(parentCtx, pending, "cancelled")
	after := time.Now()

	require.NotNil(t, transport.notifyCtx)
	require.Equal(t, MethodCancelled, transport.method)
	params, ok := transport.params.(CancelledParams)
	require.True(t, ok)
	require.JSONEq(t, `"req-1"`, string(params.RequestID))
	require.Equal(t, "trace-123", transport.notifyCtx.Value(traceKey{}))
	require.NoError(t, transport.notifyErr, "cancel notify context must be detached from parent cancellation")
	deadline, ok := transport.notifyCtx.Deadline()
	require.True(t, ok, "expected bounded cancel notification context")
	require.True(t, deadline.After(before))
	require.True(t, deadline.Before(after.Add(6*time.Second)))
}

func TestConnect_EagerHandshakeSuccess(t *testing.T) {
	base := &connectCaptureTransport{
		callResult: mustDiscoveryResultJSON(t),
	}
	transport := base

	client, err := Connect(context.Background(), transport,
		quietConnectLogger(),
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, base.startCalls)
	require.Contains(t, base.notificationMethods, MethodProgress)
	require.Equal(t, MethodServerDiscover, base.calledMethod)
	require.Empty(t, base.notifiedMethod)
	require.True(t, client.ready)
	require.NotNil(t, client.server.Meta.ServerInfo)

	paramsBytes, err := json.Marshal(base.calledParams)
	require.NoError(t, err)
	var params map[string]any
	require.NoError(t, json.Unmarshal(paramsBytes, &params))
	require.NotContains(t, params, "protocolVersion")
	require.NotContains(t, params, "roots")
	meta, ok := params["_meta"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, ProtocolVersion, meta[metaProtocolVersion])
	require.Equal(t, map[string]any{}, meta[metaClientCapabilities])
	// Keep the original workspace input as a forbidden roots request, not a
	// filesystem authority advertised through discovery.
	pending, rootsErr := base.inner.PrepareRequest(
		context.Background(),
		"roots/list",
		json.RawMessage(`{"roots":["/workspace"]}`),
	)
	require.Nil(t, pending)
	require.ErrorContains(t, rootsErr, "legacy method")
	require.Len(t, base.inner.requests, 1)

	require.NoError(t, client.Close())
	require.Equal(t, 1, base.closeCalls)
	t.Run("original unsupported selection", func(t *testing.T) {
		// The old fixture's version must now fail exact discovery, not negotiate.
		legacy := &connectCaptureTransport{
			callResult: json.RawMessage(
				`{"resultType":"complete","supportedVersions":["2024-11-05"],"capabilities":{},"ttlMs":0,"cacheScope":"private","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"test-server","version":"1.0.0"}}}`,
			),
		}
		client, err := Connect(context.Background(), legacy, quietConnectLogger())
		var versionErr *ProtocolVersionError
		require.ErrorAs(t, err, &versionErr)
		require.Nil(t, client)
		require.Equal(t, "2024-11-05", versionErr.Selected)
		require.Equal(t, MethodServerDiscover, legacy.calledMethod)
		require.Empty(t, legacy.notifiedMethod)
		require.Equal(t, 1, legacy.closeCalls)
	})
}

func TestConnect_StartFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		startErr: errors.New("start failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport, quietConnectLogger())
	require.Error(t, err)
	require.ErrorIs(t, err, base.startErr)
	require.Nil(t, client)
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_DiscoveryCallFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callErr: errors.New("initialize call failed"),
	}
	transport := base

	client, err := Connect(context.Background(), transport, quietConnectLogger())
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, MethodServerDiscover, base.calledMethod)
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_DiscoveryParseFailureClosesTransport(t *testing.T) {
	base := &connectCaptureTransport{
		callResult: []byte(`{"invalid_json"`),
	}
	transport := base

	client, err := Connect(context.Background(), transport, quietConnectLogger())
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "server/discover result")
	require.Equal(t, 1, base.closeCalls)
}

func TestConnect_LegacyInitializedNotificationIsAbsent(t *testing.T) {
	// Arrange. The original notification failure is irrelevant to current discovery.
	base := &connectCaptureTransport{callResult: mustDiscoveryResultJSON(t), notifyErr: errors.New("notify failed")}
	// Act.
	client, err := Connect(context.Background(), base, quietConnectLogger())
	// Assert.
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Empty(t, base.notifiedMethod)
	require.Nil(t, base.notifiedParams)
	require.NoError(t, client.Close())
	require.Equal(t, 1, base.closeCalls)
}

func TestToolResult_ErrorChunkPreservesTypedRemoteEnvelope(t *testing.T) {
	// Arrange.
	raw, err := json.Marshal(CallToolResult{ResultType: ResultTypeComplete,
		Content: []ContentBlock{{Type: "text", Text: "permission denied"}}, IsError: true})
	require.NoError(t, err)
	// Act.
	got, err := buildToolResultChunk("remote", raw, nil)
	// Assert.
	require.NoError(t, err)
	require.True(t, got.IsError)
	require.Equal(t, toolsy.MimeTypeText, got.MimeType)
	require.Equal(t, "permission denied", string(got.Data))
	require.NotNil(t, got.Envelope)
	require.Equal(t, toolsy.CodeRemoteExecution, got.Envelope.Error.Code)
	var remote *RemoteToolError
	require.ErrorAs(t, got.Envelope.Error, &remote)
	require.Equal(t, "permission denied", remote.Result.Content[0].Text)
	require.True(t, remote.Result.IsError)
	require.JSONEq(t, string(raw), string(mustJSON(t, got.TypedResult)))
}

func TestHandleToolCallResult_ReadLimitExceeded_MapsValidation(t *testing.T) {
	client := &Client{}
	err := client.mapCallReadLimitFor(context.Background(), textprocessor.ErrReadLimitExceeded, "MCP tool response")
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestGetResourceTool_ReadLimitExceeded(t *testing.T) {
	client := newReadyCaptureClient(t, &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded})
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
	client := newReadyCaptureClient(t, &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded})
	_, err := client.GetPrompt(context.Background(), "greeting", nil)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", httptool.DefaultMaxSSEStreamBytes))
}

func TestGetResourceTool_ReadLimit_CustomStreamCap(t *testing.T) {
	const customCap = 2048
	client := newReadyCaptureClient(t, &streamCapCaptureTransport{
		callErr:   textprocessor.ErrReadLimitExceeded,
		streamCap: customCap,
	})
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
	client := newReadyCaptureClient(t, &streamCapCaptureTransport{
		connectCaptureTransport: connectCaptureTransport{},
		streamCap:               customCap,
	})
	err := client.mapCallReadLimitFor(context.Background(), textprocessor.ErrReadLimitExceeded, "MCP tool response")
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, fmt.Sprintf("%d byte limit", customCap))
}

func TestGetTools_RemoteAnnotationsRemainUntrusted(t *testing.T) {
	toolsList := ToolsListResult{
		ResultType: ResultTypeComplete,
		TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
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

	client := newReadyCaptureClient(t, &connectCaptureTransport{callResult: resultBytes})
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
	require.False(t, readTool.Manifest().ReadOnly)
	require.True(t, readTool.Manifest().Dangerous)

	deleteTool := byName["delete_tool"]
	require.NotNil(t, deleteTool)
	require.True(t, deleteTool.Manifest().Dangerous)
	require.False(t, deleteTool.Manifest().Idempotent)
	require.False(t, deleteTool.Manifest().ReadOnly)
}

func TestConnect_Discovery_ReadLimitMapsValidation(t *testing.T) {
	t.Parallel()
	base := &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded}
	_, err := Connect(context.Background(), base, quietConnectLogger())
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "byte limit")
	require.Contains(t, te.Reason, "MCP server discovery response")
	require.Equal(t, MethodServerDiscover, base.calledMethod)
}

func TestGetTools_ReadLimitExceeded(t *testing.T) {
	t.Parallel()
	client := newReadyCaptureClient(t, &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded})
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
	client := newReadyCaptureClient(t, &connectCaptureTransport{callErr: textprocessor.ErrReadLimitExceeded})
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

func TestConnect_Discovery_ReadLimit_CustomStreamCap(t *testing.T) {
	t.Parallel()
	const customCap = 2048
	base := &streamCapCaptureTransport{
		callErr:   textprocessor.ErrReadLimitExceeded,
		streamCap: customCap,
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
	client := newReadyCaptureClient(t, &streamCapCaptureTransport{
		callErr:   textprocessor.ErrReadLimitExceeded,
		streamCap: customCap,
	})
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
	client := newReadyCaptureClient(t, &streamCapCaptureTransport{
		callErr:   textprocessor.ErrReadLimitExceeded,
		streamCap: customCap,
	})
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

func TestClient_ListBoundariesMapReadLimitsAndPreserveInterrupts(t *testing.T) {
	// Arrange. Exercise public APIs, not only the common mapping helper.
	methods := []struct {
		name, subject string
		call          func(context.Context, *Client) error
	}{
		{
			"resources",
			"MCP resources list response",
			func(ctx context.Context, c *Client) error { _, err := c.ListResources(ctx, ""); return err },
		},
		{
			"templates",
			"MCP resource templates list response",
			func(ctx context.Context, c *Client) error { _, err := c.ListResourceTemplates(ctx, ""); return err },
		},
		{
			"prompts",
			"MCP prompts list response",
			func(ctx context.Context, c *Client) error { _, err := c.ListPrompts(ctx, ""); return err },
		},
	}
	for _, method := range methods {
		for _, cap := range []int{0, 2048} {
			t.Run(fmt.Sprintf("%s/cap%d", method.name, cap), func(t *testing.T) {
				transport := &streamCapCaptureTransport{
					callErr:   textprocessor.ErrReadLimitExceeded,
					streamCap: cap,
				}
				client := newReadyCaptureClient(t, transport)
				// Act.
				err := method.call(context.Background(), client)
				// Assert.
				toolErr, ok := toolsy.AsToolError(err)
				require.True(t, ok, "expected typed read-limit failure, got %v", err)
				require.Equal(t, toolsy.CodeValidationFailed, toolErr.Code)
				require.Contains(t, toolErr.Reason, method.subject)
				limit := cap
				if limit == 0 {
					limit = httptool.DefaultMaxSSEStreamBytes
				}
				require.Contains(t, toolErr.Reason, fmt.Sprintf("%d byte limit", limit))
				require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
			})
		}
		t.Run(method.name+"/interrupt", func(t *testing.T) {
			cause := errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded)
			client := newReadyCaptureClient(t, &connectCaptureTransport{callErr: cause})
			err := method.call(context.Background(), client)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
			_, mapped := toolsy.AsToolError(err)
			require.False(t, mapped, "an interrupt must not become a validation failure")
		})
	}
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
	err := client.mapCallReadLimitFor(ctx, textprocessor.ErrReadLimitExceeded, "MCP tool response")
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
	err := client.mapCallReadLimitFor(context.Background(), composite, "MCP tool response")
	require.ErrorIs(t, err, context.Canceled)
}
