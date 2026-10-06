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

type progressTerminalTransport struct {
	*fakeTransport

	wireDone  chan struct{}
	awaitGate chan struct{}
}

type progressTerminalPrepared struct {
	*preparedRequest

	wireDone  chan struct{}
	awaitGate chan struct{}
	once      sync.Once
}

func (p *progressTerminalPrepared) OnComplete(hook func()) {
	p.preparedRequest.OnComplete(func() {
		hook()
		p.once.Do(func() { close(p.wireDone) })
	})
}

func (p *progressTerminalPrepared) Await(ctx context.Context) (json.RawMessage, error) {
	result, err := p.preparedRequest.Await(ctx)
	if err != nil {
		return result, err
	}
	select {
	case <-p.awaitGate:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *progressTerminalTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	pending, err := t.fakeTransport.PrepareRequest(ctx, method, params)
	if err != nil || method != MethodToolsCall {
		return pending, err
	}
	return &progressTerminalPrepared{
		preparedRequest: pending.(*preparedRequest),
		wireDone:        t.wireDone,
		awaitGate:       t.awaitGate,
	}, nil
}

type noProgressCompletionTransport struct{ *fakeTransport }
type noProgressCompletionPrepared struct{ PreparedRequest }

func (t *noProgressCompletionTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	pending, err := t.fakeTransport.PrepareRequest(ctx, method, params)
	if err != nil {
		return nil, err
	}
	return &noProgressCompletionPrepared{PreparedRequest: pending}, nil
}

func migrationToolScript(t *testing.T, transport *fakeTransport, onCall func(capturedRequest)) {
	t.Helper()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(t, ToolsListResult{
				ResultType: ResultTypeComplete,
				TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
				Tools: []MCPTool{{Name: "progress", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			}), nil, true
		case MethodToolsCall:
			if onCall != nil {
				onCall(request)
			}
			return mustJSON(t, CallToolResult{ResultType: ResultTypeComplete, Content: []ContentBlock{}}), nil, true
		default:
			t.Errorf("unexpected request method %q", request.Method)
			return nil, ErrTransportClosed, true
		}
	}
}

func migrationProgressProxy(ctx context.Context, t *testing.T, client *Client) toolsy.Tool {
	t.Helper()
	var proxy toolsy.Tool
	for item, err := range client.Discover(ctx) {
		require.NoError(t, err)
		proxy = item
	}
	require.NotNil(t, proxy)
	return proxy
}

func TestMigratedProgressRetiresBeforeAwaitAndDrainsAcceptedEvents(t *testing.T) {
	// Arrange. Await deliberately cannot finish until after the late frame.
	transport := &progressTerminalTransport{
		fakeTransport: newFakeTransport(),
		wireDone:      make(chan struct{}),
		awaitGate:     make(chan struct{}),
	}
	var token ProgressToken
	migrationToolScript(t, transport.fakeTransport, func(request capturedRequest) {
		var params ToolsCallParams
		require.NoError(t, json.Unmarshal(request.Params, &params))
		token = params.Meta.ProgressToken
		require.NoError(
			t,
			transport.emit(
				MethodProgress,
				ProgressParams{ProgressToken: token, Progress: 1.5, Total: new(3.5), Message: "first"},
			),
		)
		require.NoError(
			t,
			transport.emit(
				MethodProgress,
				ProgressParams{ProgressToken: token, Progress: 1, Total: new(3.5), Message: "regression"},
			),
		)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Connect(ctx, transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	proxy := migrationProgressProxy(ctx, t, client)
	var chunks []toolsy.Chunk
	finished := make(chan error, 1)

	// Act.
	go func() {
		finished <- proxy.Execute(ctx, toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: []byte(`{}`)}, func(chunk toolsy.Chunk) error { chunks = append(chunks, chunk); return nil })
	}()
	select {
	case <-transport.wireDone:
	case <-ctx.Done():
		t.Fatal("wire-terminal hook did not run")
	}
	require.NoError(
		t,
		transport.emit(
			MethodProgress,
			ProgressParams{ProgressToken: token, Progress: 2, Total: new(3.5), Message: "late-after-terminal"},
		),
	)
	close(transport.awaitGate)

	// Assert.
	require.NoError(t, <-finished)
	require.Len(t, chunks, 2)
	require.Equal(t, toolsy.EventProgress, chunks[0].Event)
	require.Equal(t, "first", chunks[0].Progress.Message)
	require.InDelta(t, 1.5, *chunks[0].Progress.Current, 0)
	require.InDelta(t, 3.5, *chunks[0].Progress.Total, 0)
	require.Equal(t, toolsy.EventResult, chunks[1].Event)
	require.True(t, chunks[1].EmptyResult)
	require.Empty(t, chunks[1].Data)
	require.Empty(t, chunks[1].MimeType)
	require.NotNil(t, chunks[1].Envelope)
	require.Empty(t, chunks[1].Envelope.MimeType)
	active := false
	client.progressCallbacks.Range(func(any, any) bool { active = true; return false })
	require.False(t, active)
}

func TestMigratedMCPConsumerAbortPreservesCause(t *testing.T) {
	for _, event := range []toolsy.EventType{toolsy.EventProgress, toolsy.EventResult} {
		t.Run(string(event), func(t *testing.T) {
			// Arrange.
			transport := newFakeTransport()
			migrationToolScript(t, transport, func(request capturedRequest) {
				if event != toolsy.EventProgress {
					return
				}
				var params ToolsCallParams
				require.NoError(t, json.Unmarshal(request.Params, &params))
				require.NoError(
					t,
					transport.emit(
						MethodProgress,
						ProgressParams{ProgressToken: params.Meta.ProgressToken, Progress: 1},
					),
				)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client, err := Connect(ctx, transport)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			proxy := migrationProgressProxy(ctx, t, client)
			cause := errors.New("consumer disconnected")
			yields := 0

			// Act.
			err = proxy.Execute(
				ctx,
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
				func(chunk toolsy.Chunk) error {
					yields++
					require.Equal(t, event, chunk.Event)
					return cause
				},
			)

			// Assert.
			require.ErrorIs(t, err, toolsy.ErrStreamAborted)
			require.ErrorIs(t, err, cause)
			require.Equal(t, 1, yields)
		})
	}
}

func TestMigratedEmptyResourceChunksRespectCoreContract(t *testing.T) {
	for _, contents := range [][]ResourceContents{
		{},
		{{URI: "file:///empty", Text: new("")}},
		{{URI: "file:///empty", Blob: new("")}},
	} {
		// Arrange.
		result := ResourcesReadResult{
			ResultType: ResultTypeComplete,
			TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
			Contents: contents,
		}
		chunk, err := buildResourceResultChunk(result, nil)
		require.NoError(t, err)
		proxy, err := toolsy.NewProxyTool(
			"empty",
			"empty",
			[]byte(`{"type":"object"}`),
			func(_ context.Context, _ *toolsy.RunEnv, _ []byte, out func(toolsy.Chunk) error) error {
				return out(chunk)
			},
		)
		require.NoError(t, err)
		yields := 0

		// Act.
		err = proxy.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(actual toolsy.Chunk) error {
				yields++
				require.Empty(t, actual.Data)
				require.Empty(t, actual.MimeType)
				require.NotNil(t, actual.Envelope)
				require.Empty(t, actual.Envelope.MimeType)
				require.IsType(t, ResourcesReadResult{}, actual.TypedResult)
				return nil
			},
		)

		// Assert.
		require.NoError(t, err)
		require.Equal(t, 1, yields)
	}
}

func TestMigratedProgressMissingCompletionAbortsBeforeDelivery(t *testing.T) {
	// Arrange.
	transport := &noProgressCompletionTransport{fakeTransport: newFakeTransport()}
	migrationToolScript(t, transport.fakeTransport, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Connect(ctx, transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	proxy := migrationProgressProxy(ctx, t, client)
	yields := 0

	// Act.
	err = proxy.Execute(
		ctx,
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { yields++; return nil },
	)

	// Assert.
	var unsupported *UnsupportedFeatureError
	require.ErrorAs(t, err, &unsupported)
	require.Equal(t, "request-scoped progress lifecycle", unsupported.Feature)
	require.Zero(t, yields)
	require.Len(t, transport.preparations, 3)
	require.Len(t, transport.requests, 2, "tools/call was prepared but never delivered")
	require.Empty(t, transport.notifications)
	_, err = transport.pending["3"].Await(ctx)
	require.ErrorAs(t, err, &unsupported)
}
