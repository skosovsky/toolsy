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

func TestConnect_ExactVersionWithoutRootsAuthority(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		require.Equal(t, MethodServerDiscover, request.Method)
		var params DiscoverParams
		require.NoError(t, json.Unmarshal(request.Params, &params))
		require.Equal(t, ProtocolVersion, params.Meta.ProtocolVersion)
		require.Nil(t, params.Meta.ClientCapabilities.Roots)
		require.NotContains(t, string(request.Params), "workspace")
		return completeDiscovery(ServerCapabilities{}), nil, true
	}
	// Act.
	client, err := Connect(context.Background(), transport)
	// Assert.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.Len(t, transport.requests, 1)
	require.Empty(t, transport.notifications)
	assertLegacyRootsRejected(t, []string{"./workspace with space"})
}

func TestConnect_DiscoveryPublishesReadyWithoutLegacyNotification(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	notifications := 0
	transport.notifyHook = func(capturedNotification) { notifications++ }
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		var invalid *InvalidPayloadError
		require.ErrorAs(
			t,
			transport.peer.dispatch([]byte(`{"jsonrpc":"2.0","id":"fast","method":"roots/list","params":{}}`)),
			&invalid,
		)
		return completeDiscovery(ServerCapabilities{}), nil, true
	}
	// Act.
	client, err := Connect(context.Background(), transport)
	// Assert.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.True(t, client.ready)
	require.Zero(t, notifications)
	require.Empty(t, transport.notifications)
	require.Empty(t, transport.peerWrites)
}

// Filesystem-root normalization is host-owned, not an MCP client capability.
// Keep every original URI/path fixture as an explicit no-authority negative.
func assertLegacyRootsRejected(t *testing.T, inputs []string) {
	t.Helper()
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	notifications := 0
	transport.peer.setNotificationHandler("roots/list", func(json.RawMessage) { notifications++ })
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// Arrange.
			raw, err := json.Marshal(map[string]any{"root": input})
			require.NoError(t, err)
			// Act.
			pending, requestErr := transport.PrepareRequest(context.Background(), "roots/list", json.RawMessage(raw))
			incomingErr := transport.peer.dispatch(
				fmt.Appendf(nil, `{"jsonrpc":"2.0","id":7,"method":"roots/list","params":%s}`, raw),
			)
			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, requestErr, &invalid)
			require.ErrorAs(t, incomingErr, &invalid)
			require.Nil(t, pending)
			require.Empty(t, transport.preparations)
			require.Empty(t, transport.requests)
			require.Empty(t, transport.notifications)
			require.Empty(t, transport.peerWrites)
			require.Zero(t, notifications)
		})
	}
}

func TestLegacyRootsRejectNonCanonicalTypedFileURIs(t *testing.T) {
	assertLegacyRootsRejected(t, []string{"file:/workspace", "file:///workspace?query=1", "file:///workspace#fragment"})
}
func TestLegacyRootsDoNotCanonicalizeTypedFilePath(t *testing.T) {
	assertLegacyRootsRejected(t, []string{"file:///workspace/dir/../root/"})
}
func TestLegacyRootsDoNotConvertWindowsDrivePaths(t *testing.T) {
	assertLegacyRootsRejected(t, []string{`C:\workspace\project`, `C:\`, `C:\..\..\etc`})
}
func TestLegacyRootsRejectUNCPaths(t *testing.T) {
	assertLegacyRootsRejected(t, []string{`\\server\share`, `//server/share`})
}
func TestLegacyRootsDoNotCanonicalizeTypedDriveURIs(t *testing.T) {
	assertLegacyRootsRejected(
		t,
		[]string{"file:///C:/../../etc", "file:///c:/workspace%5C..%5Csecret", "file:///c:%5Cworkspace%5C..%5Csecret"},
	)
}
func TestLegacyRootsRejectDriveRelativeTypedURIs(t *testing.T) {
	assertLegacyRootsRejected(t, []string{"file:///C:", "file:///C:relative/path", "file:///c%3Arelative/path"})
}
func TestLegacyRootsRejectTypedUNCAliases(t *testing.T) {
	assertLegacyRootsRejected(
		t,
		[]string{"file:////server/share", "file:///%5C%5Cserver%5Cshare", "file:///%2F%2Fserver/share"},
	)
}

func TestClient_LegacyPingRejectedBeforeAndAfterDiscovery(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return completeDiscovery(ServerCapabilities{}), nil, true
	}
	callbacks := 0
	transport.peer.setNotificationHandler("ping", func(json.RawMessage) { callbacks++ })
	// Act.
	before, beforeErr := transport.PrepareRequest(context.Background(), "ping", migrationDiscoveryParams())
	beforeIncomingErr := transport.peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`))
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	after, afterErr := transport.PrepareRequest(context.Background(), "ping", migrationDiscoveryParams())
	afterIncomingErr := transport.peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`))
	// Assert.
	require.Nil(t, before)
	require.Nil(t, after)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, beforeErr, &invalid)
	require.ErrorAs(t, afterErr, &invalid)
	require.ErrorAs(t, beforeIncomingErr, &invalid)
	require.ErrorAs(t, afterIncomingErr, &invalid)
	require.Zero(t, callbacks)
	require.Empty(t, transport.peerWrites)
	require.Len(t, transport.requests, 1, "only discovery may reach the wire")
}

func TestClient_LegacyPingAndRootsRejectMalformedRequestMeta(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	for _, method := range []string{"ping", "roots/list"} {
		t.Run(method, func(t *testing.T) {
			callbacks := 0
			transport.peer.setNotificationHandler(method, func(json.RawMessage) { callbacks++ })
			// Act.
			pending, err := transport.PrepareRequest(context.Background(), method, json.RawMessage(`{"_meta":null}`))
			incomingErr := transport.peer.dispatch(
				fmt.Appendf(nil, `{"jsonrpc":"2.0","id":1,"method":%q,"params":{"_meta":null}}`, method),
			)
			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.ErrorAs(t, incomingErr, &invalid)
			require.Zero(t, callbacks)
			require.Empty(t, transport.peerWrites)
			require.Nil(t, pending)
			require.Empty(t, transport.preparations)
			require.Empty(t, transport.requests)
		})
	}
}

func TestConnect_VersionMismatchFailsWithoutFallback(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		require.Equal(t, MethodServerDiscover, request.Method)
		result, err := json.Marshal(DiscoverResult{
			ResultType:        ResultTypeComplete,
			SupportedVersions: []string{"2025-06-18"},
			TTLMS:             JSONNumber("0"), CacheScope: CacheScopePrivate,
			Meta: ResultMeta{ServerInfo: &Implementation{Name: "old", Version: "1"}},
		})
		return result, err, true
	}
	// Act.
	client, err := Connect(context.Background(), transport)
	// Assert.
	require.Nil(t, client)
	var versionErr *ProtocolVersionError
	require.ErrorAs(t, err, &versionErr)
	require.Len(t, transport.requests, 1)
	require.Empty(t, transport.notifications)
	require.True(t, transport.closed)
}

func TestClient_InputRootsSnapshotOwnsMetadataWithoutRootsService(t *testing.T) {
	// Arrange. Roots are an explicit host-owned input response, not a callback.
	input := json.RawMessage(`{"roots":{"roots":[{"uri":"file:///workspace","_meta":{"origin":{"owner":"caller"}}}]}}`)
	client := &Client{opts: ClientOptions{ClientInfo: Implementation{Name: "client", Version: "test"}}}
	// Act.
	snapshot, err := client.prepareParams(ToolsCallParams{Name: "sample", InputResponses: input})
	require.NoError(t, err)
	copy(input, []byte(`{"roots":{"roots":[{"uri":"file:///workspace","_meta":{"attacker":{"owner":"mutate"}}}]}}`))
	// Assert.
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(snapshot, &wire))
	var responses map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire["inputResponses"], &responses))
	var roots struct {
		Roots []struct {
			URI  string `json:"uri"`
			Meta Meta   `json:"_meta"`
		} `json:"roots"`
	}
	require.NoError(t, json.Unmarshal(responses["roots"], &roots))
	require.Len(t, roots.Roots, 1)
	require.JSONEq(t, `{"owner":"caller"}`, string(roots.Roots[0].Meta["origin"]))
	require.NotContains(t, roots.Roots[0].Meta, "attacker")
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
	pending, _, err := peer.beginRequest("completed", struct{}{})
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
	// Act.
	err := validatePrompt(Prompt{Name: "daily standup"})

	// Assert.
	require.NoError(t, err)
}

func TestClient_MissingCapabilityRejectedWithoutRPC(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(_ capturedRequest) (json.RawMessage, error, bool) {
		return completeDiscovery(ServerCapabilities{}), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	requestCount := len(transport.requests)

	// Act.
	var gotErr error
	for _, iterErr := range client.Discover(context.Background()) {
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
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					ResultType: ResultTypeComplete,
					TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
					Tools: []MCPTool{{
						Name: "sample.tool",
						InputSchema: json.RawMessage(
							`{"type":"object","properties":{},"additionalProperties":false}`,
						),
						OutputSchema: json.RawMessage(
							`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`,
						),
					}},
				},
			), nil, true
		case MethodToolsCall:
			return mustJSON(t, CallToolResult{ResultType: ResultTypeComplete,
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
	for item, iterErr := range client.Discover(context.Background()) {
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
	require.IsType(t, CallToolResult{ResultType: ResultTypeComplete}, chunk.TypedResult)
	require.NotNil(t, chunk.Envelope)
	require.Equal(t, toolsy.ToolEnvelopeKindResult, chunk.Envelope.Kind)
}

func TestClient_ToolResultViolatingOutputSchemaFailsClosed(t *testing.T) {
	// Arrange.
	raw := mustJSON(t, CallToolResult{ResultType: ResultTypeComplete,
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

func TestClient_OutputSchemaPreservesExactNumericConstraints(t *testing.T) {
	// Arrange.
	validator, err := compileOutputSchema(json.RawMessage(
		`{"type":"object","properties":{"n":{"type":"integer","minimum":9007199254740993}},"required":["n"]}`,
	))
	require.NoError(t, err)
	raw := mustJSON(t, CallToolResult{ResultType: ResultTypeComplete,
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
		CallToolResult{
			ResultType: ResultTypeComplete,
			Content:    []ContentBlock{{Type: "text", Text: "bad input"}},
			IsError:    true,
		},
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
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					ResultType: ResultTypeComplete,
					TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
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
	for item, iterErr := range client.Discover(context.Background()) {
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
	// The Await and caller-context paths race for one notification owner. Wait
	// for the bounded detached send, rather than assuming which path won.
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		for _, notification := range transport.notifications {
			if notification.Method == MethodCancelled {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	transport.mu.Lock()
	notifications := append([]capturedNotification(nil), transport.notifications...)
	transport.mu.Unlock()
	var cancellations []CancelledParams
	for _, notification := range notifications {
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
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					ResultType: ResultTypeComplete,
					TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
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
						Progress:      1.5,
						Total:         new(3.5),
						Message:       "first",
					},
				)
				transport.emit(
					MethodProgress,
					ProgressParams{
						ProgressToken: params.Meta.ProgressToken,
						Progress:      1.0,
						Total:         new(3.5),
						Message:       "regression",
					},
				)
				transport.complete(
					string(request.ID),
					CallToolResult{
						ResultType: ResultTypeComplete,
						Content:    []ContentBlock{{Type: "text", Text: "done"}},
					},
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
	for item, iterErr := range client.Discover(context.Background()) {
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
	require.Len(t, progress, 1)
	require.InDelta(t, 1.5, *progress[0].Current, 0.0001)
	require.InDelta(t, 3.5, *progress[0].Total, 0.0001)
	require.Equal(t, "first", progress[0].Message)
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
