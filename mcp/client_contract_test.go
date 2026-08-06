package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type stalledDeliveryPending struct{ delivery chan struct{} }

func (p *stalledDeliveryPending) ID() json.RawMessage { return json.RawMessage(`99`) }
func (p *stalledDeliveryPending) Await(context.Context) (json.RawMessage, error) {
	return nil, context.Canceled
}
func (p *stalledDeliveryPending) DeliveryDone() <-chan struct{} { return p.delivery }
func (p *stalledDeliveryPending) WasSent() bool                 { return false }
func (p *stalledDeliveryPending) CancelPending() bool           { return true }

func TestConnect_ExactVersionAndCanonicalRoots(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		require.Equal(t, MethodInitialize, request.Method)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(request.Params, &raw))
		require.NotContains(t, raw, "roots")
		var params InitializeParams
		require.NoError(t, json.Unmarshal(request.Params, &params))
		require.Equal(t, ProtocolVersion, params.ProtocolVersion)
		require.NotNil(t, params.Capabilities.Roots)
		require.False(t, params.Capabilities.Roots.ListChanged)
		return initializeResult(ServerCapabilities{}), nil, true
	}

	// Act.
	client, err := Connect(
		context.Background(),
		transport,
		WithClientRoots([]string{"./workspace with space"}),
	)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, ProtocolVersion, transport.version)
	result, rpcErr := transport.handler(
		context.Background(),
		Request{Method: MethodRootsList, ID: json.RawMessage("7")},
	)
	require.Nil(t, rpcErr)
	var roots RootsListResult
	require.NoError(t, json.Unmarshal(result, &roots))
	require.Len(t, roots.Roots, 1)
	require.Contains(t, roots.Roots[0].URI, "file://")
	require.Contains(t, roots.Roots[0].URI, "workspace%20with%20space")
	require.NoError(t, client.Close())
}

func TestConnect_PublishesOperationStateOnlyAfterInitializedNotificationReturns(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{}), nil, request.Method == MethodInitialize
	}
	notifyStarted := make(chan struct{})
	releaseNotify := make(chan struct{})
	transport.notifyHook = func(notification capturedNotification) {
		if notification.Method != MethodInitialized {
			return
		}
		close(notifyStarted)
		<-releaseNotify
	}
	type connectResult struct {
		client *Client
		err    error
	}
	connected := make(chan connectResult, 1)

	// Act.
	go func() {
		client, err := Connect(
			context.Background(),
			transport,
			WithRoots([]Root{{URI: "file:///workspace"}}),
		)
		connected <- connectResult{client: client, err: err}
	}()
	<-notifyStarted
	_, beforeErr := transport.handler(
		context.Background(),
		Request{JSONRPC: JSONRPCVersion, ID: json.RawMessage(`"before"`), Method: MethodRootsList},
	)
	close(releaseNotify)
	result := <-connected
	_, afterErr := transport.handler(
		context.Background(),
		Request{JSONRPC: JSONRPCVersion, ID: json.RawMessage(`"after"`), Method: MethodRootsList},
	)

	// Assert.
	require.NotNil(t, beforeErr)
	require.Equal(t, JSONRPCMethodNotFound, beforeErr.Code)
	require.NoError(t, result.err)
	require.Nil(t, afterErr)
	require.NoError(t, result.client.Close())
}

func TestConnect_AllowsSchemaValidEmptyImplementationStrings(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	clientInfo := Implementation{Name: " ", Version: ""}
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		var params InitializeParams
		require.NoError(t, json.Unmarshal(request.Params, &params))
		require.Equal(t, clientInfo, params.ClientInfo)
		return mustJSON(t, InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities:    ServerCapabilities{},
			ServerInfo:      Implementation{Name: "", Version: " "},
		}), nil, true
	}

	// Act.
	client, err := Connect(context.Background(), transport, WithClientInfo(clientInfo))

	// Assert.
	require.NoError(t, err)
	require.NoError(t, client.Close())
}

func TestClient_AllowsSchemaValidEmptyPromptAndArgumentNames(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Prompts: &PromptsCapability{}}), nil, true
		case MethodPromptsList:
			return mustJSON(t, PromptsListResult{Prompts: []Prompt{{
				Name: "", Arguments: []PromptArgument{{Name: " "}},
			}}}), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	defer client.Close()

	// Act.
	var prompts []Prompt
	for prompt, iterErr := range client.GetPrompts(context.Background()) {
		require.NoError(t, iterErr)
		prompts = append(prompts, prompt)
	}

	// Assert.
	require.Equal(t, []Prompt{{Name: "", Arguments: []PromptArgument{{Name: " "}}}}, prompts)
}

func TestNormalizeRootsRejectsNonCanonicalTypedFileURIs(t *testing.T) {
	// Arrange.
	invalid := []string{
		"file:/workspace",
		"file:///workspace?query=1",
		"file:///workspace#fragment",
	}

	for _, rootURI := range invalid {
		// Act.
		roots, err := normalizeRoots(nil, []Root{{URI: rootURI}})

		// Assert.
		require.Nil(t, roots)
		require.Error(t, err)
	}
}

func TestNormalizeRootsCanonicalizesTypedFilePath(t *testing.T) {
	// Arrange.
	root := Root{URI: "file:///workspace/dir/../root/"}

	// Act.
	roots, err := normalizeRoots(nil, []Root{root})

	// Assert.
	require.NoError(t, err)
	require.Equal(t, "file:///workspace/root", roots[0].URI)
}

func TestNormalizeRootsConvertsWindowsDrivePathPortably(t *testing.T) {
	// Arrange.
	tests := []struct {
		path string
		uri  string
		name string
	}{
		{path: `C:\workspace\project`, uri: "file:///C:/workspace/project", name: "project"},
		{path: `C:\`, uri: "file:///C:/", name: "C:"},
		{path: `C:\..\..\etc`, uri: "file:///C:/etc", name: "etc"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots([]string{test.path}, nil)

			// Assert.
			require.NoError(t, err)
			require.Equal(t, test.uri, roots[0].URI)
			require.Equal(t, test.name, roots[0].Name)
		})
	}
}

func TestNormalizeRootsRejectsUNCPath(t *testing.T) {
	// Arrange.
	for _, rootPath := range []string{`\\server\share`, `//server/share`} {
		t.Run(rootPath, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots([]string{rootPath}, nil)

			// Assert.
			require.Nil(t, roots)
			require.Error(t, err)
		})
	}
}

func TestNormalizeRootsKeepsDriveWhenCanonicalizingTypedURI(t *testing.T) {
	// Arrange.
	tests := []struct {
		uri  string
		want string
	}{
		{uri: "file:///C:/../../etc", want: "file:///C:/etc"},
		{uri: "file:///c:/workspace%5C..%5Csecret", want: "file:///C:/secret"},
		{uri: "file:///c:%5Cworkspace%5C..%5Csecret", want: "file:///C:/secret"},
	}
	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots(nil, []Root{{URI: test.uri}})

			// Assert.
			require.NoError(t, err)
			require.Equal(t, test.want, roots[0].URI)
		})
	}
}

func TestNormalizeRootsRejectsDriveRelativeTypedURIs(t *testing.T) {
	// Arrange.
	invalid := []string{"file:///C:", "file:///C:relative/path", "file:///c%3Arelative/path"}

	for _, rootURI := range invalid {
		t.Run(rootURI, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots(nil, []Root{{URI: rootURI}})

			// Assert.
			require.Nil(t, roots)
			require.Error(t, err)
		})
	}
}

func TestNormalizeRootsRejectsTypedUNCAliases(t *testing.T) {
	// Arrange.
	aliases := []string{
		"file:////server/share",
		"file:///%5C%5Cserver%5Cshare",
		"file:///%2F%2Fserver/share",
	}
	for _, rootURI := range aliases {
		t.Run(rootURI, func(t *testing.T) {
			// Act.
			roots, err := normalizeRoots(nil, []Root{{URI: rootURI}})

			// Assert.
			require.Nil(t, roots)
			require.Error(t, err)
		})
	}
}

func TestClient_PingWorksBeforeAndAfterInitialization(t *testing.T) {
	// Arrange.
	client := &Client{}
	request := Request{JSONRPC: JSONRPCVersion, ID: json.RawMessage(`1`), Method: MethodPing}

	// Act.
	before, beforeErr := client.handleRequest(context.Background(), request)
	client.initialized = true
	after, afterErr := client.handleRequest(context.Background(), request)

	// Assert.
	require.Nil(t, beforeErr)
	require.Nil(t, afterErr)
	require.JSONEq(t, `{}`, string(before))
	require.JSONEq(t, `{}`, string(after))
}

func TestClient_PingAndRootsRejectMalformedRequestMeta(t *testing.T) {
	// Arrange.
	client := &Client{initialized: true, roots: []Root{{URI: "file:///workspace"}}}
	methods := []string{MethodPing, MethodRootsList}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			request := Request{
				JSONRPC: JSONRPCVersion,
				ID:      json.RawMessage(`1`),
				Method:  method,
				Params:  json.RawMessage(`{"_meta":null}`),
			}

			// Act.
			result, rpcErr := client.handleRequest(context.Background(), request)

			// Assert.
			require.Nil(t, result)
			require.NotNil(t, rpcErr)
			require.Equal(t, JSONRPCInvalidParams, rpcErr.Code)
		})
	}
}

func TestClient_CancellationDeliveryWaitIsBounded(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	client := &Client{transport: transport}
	pending := &stalledDeliveryPending{delivery: make(chan struct{})}
	started := time.Now()

	// Act.
	client.notifyCancelledPending(context.Background(), pending, "cancelled")

	// Assert.
	require.Less(t, time.Since(started), 2*time.Second)
	require.Empty(t, transport.notifications)
}

func TestClient_DoesNotCancelTerminalPendingRequest(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest("completed", nil)
	require.NoError(t, err)
	pending.markSent()
	response := fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID())
	require.NoError(t, peer.dispatch(response))
	transport := newFakeTransport()
	client := &Client{transport: transport}

	// Act.
	client.notifyCancelledPending(context.Background(), pending, "late cancellation")

	// Assert.
	require.Empty(t, transport.notifications)
}

func TestValidatePrompt_AllowsHumanReadableName(t *testing.T) {
	// Arrange.
	prompt := Prompt{Name: "daily standup"}

	// Act.
	err := validatePrompt(prompt)

	// Assert.
	require.NoError(t, err)
}

func TestConnect_VersionMismatchFailsBeforeInitialized(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(_ capturedRequest) (json.RawMessage, error, bool) {
		result, err := json.Marshal(InitializeResult{
			ProtocolVersion: "2025-06-18",
			ServerInfo:      Implementation{Name: "old", Version: "1"},
		})
		return result, err, true
	}

	// Act.
	client, err := Connect(context.Background(), transport)

	// Assert.
	require.Nil(t, client)
	var versionErr *ProtocolVersionError
	require.ErrorAs(t, err, &versionErr)
	for _, notification := range transport.notifications {
		require.NotEqual(t, MethodInitialized, notification.Method)
	}
}

func TestClient_RootsSnapshotOwnsMetadata(t *testing.T) {
	// Arrange.
	metaRaw := json.RawMessage(`{"owner":"caller"}`)
	roots := []Root{{
		URI:  "file:///workspace",
		Meta: Meta{"origin": metaRaw},
	}}
	transport := newFakeTransport()
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{}), nil, true
	}
	client, err := Connect(context.Background(), transport, WithRoots(roots))
	require.NoError(t, err)
	copy(metaRaw, []byte(`{"owner":"mutate"}`))
	delete(roots[0].Meta, "origin")
	roots[0].Meta["attacker"] = json.RawMessage(`true`)

	// Act.
	resultRaw, rpcErr := transport.handler(context.Background(), Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  MethodRootsList,
		Params:  json.RawMessage(`{}`),
	})
	var result RootsListResult
	decodeErr := json.Unmarshal(resultRaw, &result)

	// Assert.
	require.Nil(t, rpcErr)
	require.NoError(t, decodeErr)
	require.Len(t, result.Roots, 1)
	require.JSONEq(t, `{"owner":"caller"}`, string(result.Roots[0].Meta["origin"]))
	require.NotContains(t, result.Roots[0].Meta, "attacker")
	require.NoError(t, client.Close())
}

func TestClient_MissingCapabilityRejectedWithoutRPC(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(_ capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{}), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	requestCount := len(transport.requests)

	// Act.
	var gotErr error
	for _, iterErr := range client.GetTools(context.Background()) {
		gotErr = iterErr
	}

	// Assert.
	var capabilityErr *CapabilityError
	require.ErrorAs(t, gotErr, &capabilityErr)
	require.Len(t, transport.requests, requestCount)
}

func TestClient_ToolStructuredResultMapsOutputSchemaAndTypedEnvelope(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(t, ToolsListResult{Tools: []MCPTool{{
				Name: "sample.tool",
				InputSchema: json.RawMessage(
					`{"type":"object","properties":{},"additionalProperties":false}`,
				),
				OutputSchema: json.RawMessage(
					`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`,
				),
			}}}), nil, true
		case MethodToolsCall:
			return mustJSON(t, CallToolResult{
				Content:           []ContentBlock{{Type: "text", Text: "done"}},
				StructuredContent: json.RawMessage(`{"ok":true}`),
			}), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var proxy toolsy.Tool
	for item, iterErr := range client.GetTools(context.Background()) {
		require.NoError(t, iterErr)
		proxy = item
	}
	require.NotNil(t, proxy)
	require.NotEmpty(t, proxy.Manifest().OutputSchema)

	// Act.
	var chunk toolsy.Chunk
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(result toolsy.Chunk) error {
			chunk = result
			return nil
		},
	)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, toolsy.MimeTypeJSON, chunk.MimeType)
	require.JSONEq(t, `{"ok":true}`, string(chunk.Data))
	require.IsType(t, CallToolResult{}, chunk.TypedResult)
	require.NotNil(t, chunk.Envelope)
	require.Equal(t, toolsy.ToolEnvelopeKindResult, chunk.Envelope.Kind)
}

func TestClient_ToolResultViolatingOutputSchemaFailsClosed(t *testing.T) {
	// Arrange.
	raw := mustJSON(t, CallToolResult{
		Content:           []ContentBlock{},
		StructuredContent: json.RawMessage(`{"ok":"wrong"}`),
	})
	validator, err := compileOutputSchema(
		json.RawMessage(
			`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`,
		),
	)
	require.NoError(t, err)

	// Act.
	_, err = buildToolResultChunk("sample", raw, validator)

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
}

func TestClient_MalformedBinaryToolResultNeverBecomesSuccessChunk(t *testing.T) {
	// Arrange.
	fixtures := []json.RawMessage{
		json.RawMessage(`{"content":[{"type":"image","data":"%%%","mimeType":"image/png"}]}`),
		json.RawMessage(`{"content":[{"type":"audio","data":"%%%","mimeType":"audio/mpeg"}]}`),
		json.RawMessage(`{"content":[{"type":"resource","resource":{"uri":"file:///x","blob":"%%%"}}]}`),
	}

	for _, raw := range fixtures {
		// Act.
		chunk, err := buildToolResultChunk("sample", raw, nil)

		// Assert.
		var payloadErr *InvalidPayloadError
		require.ErrorAs(t, err, &payloadErr)
		require.Equal(t, toolsy.Chunk{}, chunk)
	}
}

func TestClient_OutputSchemaPreservesExactNumericConstraints(t *testing.T) {
	// Arrange.
	validator, err := compileOutputSchema(json.RawMessage(
		`{"type":"object","properties":{"n":{"type":"integer","minimum":9007199254740993}},"required":["n"]}`,
	))
	require.NoError(t, err)
	raw := mustJSON(t, CallToolResult{
		Content:           []ContentBlock{},
		StructuredContent: json.RawMessage(`{"n":9007199254740992}`),
	})

	// Act.
	_, err = buildToolResultChunk("sample", raw, validator)

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
}

func TestClient_RemoteToolErrorHasDistinctEnvelope(t *testing.T) {
	// Arrange.
	raw := mustJSON(
		t,
		CallToolResult{Content: []ContentBlock{{Type: "text", Text: "bad input"}}, IsError: true},
	)

	// Act.
	chunk, err := buildToolResultChunk("sample", raw, nil)

	// Assert.
	require.NoError(t, err)
	require.True(t, chunk.IsError)
	require.Equal(t, toolsy.CodeRemoteExecution, chunk.Envelope.Error.Code)
	var remoteErr *RemoteToolError
	require.ErrorAs(t, chunk.Envelope.Error, &remoteErr)
}

func TestClient_CancellationUsesActiveRequestIDExactlyOnce(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					Tools: []MCPTool{
						{Name: "wait", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		case MethodToolsCall:
			return nil, nil, false
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var proxy toolsy.Tool
	for item, iterErr := range client.GetTools(context.Background()) {
		require.NoError(t, iterErr)
		proxy = item
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	// Act.
	go func() {
		result <- proxy.Execute(ctx, toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: []byte(`{}`)}, func(toolsy.Chunk) error { return nil })
	}()
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.requests) >= 3
	}, time.Second, time.Millisecond)
	cancel()

	// Assert.
	require.ErrorIs(t, <-result, context.Canceled)
	var cancellations []CancelledParams
	for _, notification := range transport.notifications {
		if notification.Method == MethodCancelled {
			var params CancelledParams
			require.NoError(t, json.Unmarshal(notification.Params, &params))
			cancellations = append(cancellations, params)
		}
	}
	require.Len(t, cancellations, 1)
	require.JSONEq(t, `3`, string(cancellations[0].RequestID))
}

func TestClient_ProgressIsFractionalAndMonotonic(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					Tools: []MCPTool{
						{Name: "progress", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		case MethodToolsCall:
			var params ToolsCallParams
			require.NoError(t, json.Unmarshal(request.Params, &params))
			go func() {
				transport.emit(
					MethodProgress,
					ProgressParams{
						ProgressToken: params.Meta.ProgressToken,
						Progress:      -2,
						Total:         new(-1.0),
						Message:       "first",
					},
				)
				transport.emit(
					MethodProgress,
					ProgressParams{
						ProgressToken: params.Meta.ProgressToken,
						Progress:      -1,
						Total:         new(-1.0),
						Message:       "second",
					},
				)
				transport.emit(
					MethodProgress,
					ProgressParams{
						ProgressToken: params.Meta.ProgressToken,
						Progress:      -1.5,
						Total:         new(-1.0),
						Message:       "regression",
					},
				)
				transport.complete(
					string(request.ID),
					CallToolResult{Content: []ContentBlock{{Type: "text", Text: "done"}}},
					nil,
				)
				transport.emit(MethodProgress, ProgressParams{
					ProgressToken: params.Meta.ProgressToken,
					Progress:      2.0,
					Total:         new(3.5),
					Message:       "late-after-terminal",
				})
			}()
			return nil, nil, false
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var proxy toolsy.Tool
	for item, iterErr := range client.GetTools(context.Background()) {
		require.NoError(t, iterErr)
		proxy = item
	}
	var mu sync.Mutex
	var progress []toolsy.ProgressInfo

	// Act.
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(chunk toolsy.Chunk) error {
			if chunk.Progress != nil {
				mu.Lock()
				progress = append(progress, *chunk.Progress)
				mu.Unlock()
			}
			return nil
		},
	)

	// Assert.
	require.NoError(t, err)
	require.Len(t, progress, 2)
	require.InDelta(t, -2, *progress[0].Current, 0.0001)
	require.InDelta(t, -1, *progress[0].Total, 0.0001)
	require.Equal(t, "first", progress[0].Message)
	require.InDelta(t, -1, *progress[1].Current, 0.0001)
	require.InDelta(t, -1, *progress[1].Total, 0.0001)
	require.Equal(t, "second", progress[1].Message)
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
