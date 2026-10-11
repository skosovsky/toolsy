package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestSDKActualHTTPRoutingHeaders(t *testing.T) {
	// Arrange: production bridge over official HTTP transports.
	server := sdk.NewServer(&sdk.Implementation{Name: "headers", Version: "1"}, nil)
	server.AddTool(
		&sdk.Tool{
			Name: "route",
			InputSchema: json.RawMessage(
				`{"type":"object","properties":{"region":{"type":"string","x-mcp-header":"Region"}},"required":["region"]}`,
			),
		},
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{StructuredContent: req.Params.Arguments}, nil
		},
	)
	headers := make(chan http.Header, 4)
	handler := sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Mcp-Method") == toolCallMethod {
			headers <- r.Header.Clone()
		}
		handler.ServeHTTP(w, r)
	}))
	defer host.Close()
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(
		ctx,
		&sdk.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client()},
		nil,
	)
	require.NoError(t, err)
	defer session.Close()
	importer, err := hooks.Bind(session, "routing-host")
	require.NoError(t, err)
	defer importer.Close()
	snapshot, err := importer.Discover(ctx)
	require.NoError(t, err)
	registry, err := toolsy.NewRegistry(snapshot.Tools...)
	require.NoError(t, err)
	// Act: schema-owned routing information reaches the actual request.
	err = registry.Execute(
		ctx,
		toolsy.ToolCall{ToolName: "route", Input: toolsy.ToolInput{ArgsJSON: []byte(`{"region":"apac"}`)}},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert: headers come from SDK, without adapter transport/parser.
	require.NoError(t, err)
	select {
	case got := <-headers:
		require.Equal(t, "route", got.Get("Mcp-Name"))
		require.Equal(t, "apac", got.Get("Mcp-Param-Region"))
		require.NotEmpty(t, got.Get("Mcp-Protocol-Version"))
	case <-ctx.Done():
		t.Fatal("no tools/call observed")
	}
}

func TestSDKInvalidationOverflowFailsClosedWithoutClosingBorrowedSession(t *testing.T) {
	// Arrange: one buffered invalidation and a host-owned connection.
	config := importConfig()
	config.UpdateBuffer = 1
	server := sdk.NewServer(&sdk.Implementation{Name: "updates", Version: "1"}, nil)
	server.AddTool(
		&sdk.Tool{Name: "one", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		},
	)
	importer, session := adapterPair(t, server, false, config, nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act: delivery capacity is exceeded.
	importer.invalidate("tools", "")
	importer.invalidate("tools", "")
	// Assert: revocation is not lost; late updates cannot resurrect state.
	require.ErrorIs(t, importer.Err(), ErrQueueFull)
	_, callErr := execute(t, snapshot.Tools[0], `{}`)
	require.ErrorIs(t, callErr, ErrQueueFull)
	importer.invalidate("tools", "")
	_, err = session.ListTools(t.Context(), nil)
	require.NoError(t, err)
}

func TestSDKUnboundSessionCompletionReleasesHookOwnership(t *testing.T) {
	// Arrange: hooks may observe SDK connections that the host never imports.
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	// Act: repeatedly connect and dispose without Bind, using actual SDK sessions.
	for range 5 {
		server := sdk.NewServer(&sdk.Implementation{Name: "unused", Version: "1"}, nil)
		a, b := sdk.NewInMemoryTransports()
		ss, serverErr := server.Connect(t.Context(), b, nil)
		require.NoError(t, serverErr)
		cs, clientErr := client.Connect(t.Context(), a, nil)
		require.NoError(t, clientErr)
		require.NoError(t, cs.Close())
		require.NoError(t, ss.Close())
	}
	// Assert: completed sessions and their cache graphs are no longer retained.
	require.Eventually(t, func() bool {
		hooks.mu.Lock()
		defer hooks.mu.Unlock()
		return len(hooks.seen) == 0 && len(hooks.sessions) == 0
	}, time.Second, time.Millisecond)
}

func TestSDKHostRoundRejectsDetectableRepresentationLossBeforeDispatch(t *testing.T) {
	for _, value := range []any{int64(9007199254740993), json.Number("9007199254740993"), json.RawMessage(`9007199254740993`), map[string]any{"nested": []any{int64(9007199254740993)}}} {
		// Arrange: host data has not entered a lossy SDK wire decoder.
		response := &sdk.ElicitResult{Action: "accept", Content: map[string]any{"value": value}}
		round := Round{RequestState: "host-state", InputResponses: sdk.InputResponseMap{"question": response}}
		original, err := json.Marshal(round)
		require.NoError(t, err)
		// Act: the adapter rejects rather than silently modifying the host response.
		_, err = WithRound(t.Context(), round)
		// Assert: no continuation context is produced; caller retains exact input.
		require.ErrorIs(t, err, ErrUnsupported)
		after, marshalErr := json.Marshal(round)
		require.NoError(t, marshalErr)
		require.Equal(t, string(original), string(after))
	}
	// Arrange / Act: a representable host round is copied and remains independent.
	response := &sdk.ElicitResult{Action: "accept", Content: map[string]any{"value": int64(7)}}
	ctx, err := WithRound(t.Context(), Round{InputResponses: sdk.InputResponseMap{"q": response}})
	require.NoError(t, err)
	response.Content["value"] = int64(99)
	// Assert: tool and resource execution take the same guarded second copy.
	owned, err := cloneRound(ctx.Value(roundKey{}).(Round))
	require.NoError(t, err)
	raw, err := json.Marshal(owned)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"value":7`)
}

type invalidatePreparedProfile struct{ importer *Importer }

func (p invalidatePreparedProfile) ExecutePrepared(
	_ context.Context,
	_ toolsy.PreparedCall,
	handler toolsy.InvocationHandler,
	yield func(toolsy.Chunk) error,
) error {
	p.importer.invalidate("resources", "")
	return handler(yield)
}

func TestSDKResourceRevokedDuringPreparationNeverDispatches(t *testing.T) {
	// Arrange: a trusted execution profile waits while resource authority is revoked.
	var dispatch atomic.Int32
	server := sdk.NewServer(&sdk.Implementation{Name: "resource-revoke", Version: "1"}, nil)
	server.AddResource(
		&sdk.Resource{URI: "resource://one", Name: "one"},
		func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			dispatch.Add(1)
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: "resource://one", Text: "ok"}}}, nil
		},
	)
	importer, _ := adapterPair(t, server, false, importConfig(), nil)
	tool, err := importer.ResourceTool()
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(invalidatePreparedProfile{importer: importer})).
		Add(tool).
		Build()
	require.NoError(t, err)
	// Act: the profile revokes generation before entering the resource handler.
	err = registry.Execute(
		t.Context(),
		toolsy.ToolCall{
			ToolName: "mcp_read_resource",
			Input:    toolsy.ToolInput{ArgsJSON: []byte(`{"uri":"resource://one"}`)},
		},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert: final dispatch gate sees revocation; no remote effect occurs.
	require.ErrorIs(t, err, ErrStale)
	require.Zero(t, dispatch.Load())
}

func TestSDKCancelledDiscoveryDoesNotPublishOrRevokeExistingGeneration(t *testing.T) {
	// Arrange: host classification cancels a later discovery while preparing proxies.
	ctx, cancel := context.WithCancel(t.Context())
	var cancelDuringClassification atomic.Bool
	config := importConfig()
	config.Classify = func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) {
		if cancelDuringClassification.Load() {
			cancel()
		}
		return nil, nil
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "cancel-discovery", Version: "1"}, nil)
	server.AddTool(
		&sdk.Tool{Name: "one", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		},
	)
	importer, _ := adapterPair(t, server, false, config, nil)
	original, err := importer.Discover(t.Context())
	require.NoError(t, err)
	cancelDuringClassification.Store(true)
	// Act: transaction cancellation happens after reading SDK catalog but before publication.
	snapshot, err := importer.Discover(ctx)
	// Assert: no catalog or generation is published and the previous authority survives.
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, snapshot.Tools)
	generation, currentErr := importer.current()
	require.NoError(t, currentErr)
	require.Equal(t, original.Generation, generation)
	_, oldErr := execute(t, original.Tools[0], `{}`)
	require.NoError(t, oldErr)
}
