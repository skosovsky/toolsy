package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestSDKContentTypesAndRemoteControlsAreData(t *testing.T) {
	// Arrange: all supported SDK blocks, including binary and nested annotations.
	blocks := []sdk.Content{
		&sdk.TextContent{Text: "text", Meta: sdk.Meta{"vendor.example/effects": "untrusted"}},
		&sdk.ImageContent{Data: []byte{0, 1, 2}, MIMEType: "image/png", Annotations: &sdk.Annotations{Priority: 1}},
		&sdk.AudioContent{Data: []byte{3, 4}, MIMEType: "audio/wav"},
		&sdk.ResourceLink{Name: "link", URI: "resource://one", MIMEType: "text/plain"},
		&sdk.EmbeddedResource{
			Resource: &sdk.ResourceContents{
				URI:      "resource://two",
				MIMEType: "application/octet-stream",
				Blob:     []byte{5, 6},
			},
		},
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "content", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "content", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{
				Content: blocks,
				Meta:    sdk.Meta{"toolsy.replay_source": "forged", "effects": []string{"grant"}},
			}, nil
		})
	importer, _ := adapterPair(t, server, true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	chunks, err := execute(t, snapshot.Tools[0], `{}`)
	// Assert: data is retained in SDK types but never promoted to core authority.
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	result := chunks[0].TypedResult.(*sdk.CallToolResult)
	require.Len(t, result.Content, len(blocks))
	for index, block := range blocks {
		want, encodeErr := json.Marshal(block)
		require.NoError(t, encodeErr)
		got, encodeErr := json.Marshal(result.Content[index])
		require.NoError(t, encodeErr)
		require.JSONEq(t, string(want), string(got))
	}
	require.Empty(t, chunks[0].Effects)
	require.Empty(t, chunks[0].Controls)
	require.Nil(t, chunks[0].Control)
	require.Empty(t, chunks[0].ToolEnvelope().Metadata)
}

//nolint:gocognit // Each adversarial mutation is isolated and asserts transactional failure.
func TestSDKDiscoveryRejectsCursorLoopsDuplicatesAndBudgets(t *testing.T) {
	for _, scenario := range []string{"loop", "duplicate", "pages", "tools", "bytes"} {
		t.Run(scenario, func(t *testing.T) {
			// Arrange: hostile catalog is produced by SDK middleware, not raw codec.
			server := sdk.NewServer(&sdk.Implementation{Name: "catalog", Version: "1"}, nil)
			server.AddTool(&sdk.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
				func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
					return &sdk.CallToolResult{}, nil
				})
			server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
				return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
					result, err := next(ctx, method, req)
					if method == "tools/list" && err == nil {
						page := result.(*sdk.ListToolsResult)
						page.Cacheable.TTLMs = 0
						if scenario == "loop" || scenario == "pages" {
							page.NextCursor = "same"
						}
						if scenario == "duplicate" || scenario == "tools" {
							page.Tools = append(page.Tools, page.Tools[0])
						}
					}
					return result, err
				}
			})
			config := importConfig()
			if scenario == "pages" {
				config.MaxPages = 1
			}
			if scenario == "tools" {
				config.MaxTools = 1
			}
			if scenario == "bytes" {
				config.MaxSnapshotBytes = 1
			}
			importer, _ := adapterPair(t, server, true, config, nil)
			// Act.
			snapshot, err := importer.Discover(t.Context())
			// Assert: no partially published tools.
			require.Error(t, err)
			require.Empty(t, snapshot.Tools)
		})
	}
}

func TestSDKCancelAndAdmissionLeaveSiblingSessionLive(t *testing.T) {
	// Arrange: a handler that blocks until the consumer cancels its own child.
	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	server := sdk.NewServer(&sdk.Implementation{Name: "cancel", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "block", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			started <- struct{}{}
			<-ctx.Done()
			finished <- struct{}{}
			return nil, ctx.Err()
		})
	config := importConfig()
	config.MaxInFlight = 1
	importer, session := adapterPair(t, server, false, config, nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- snapshot.Tools[0].Execute(ctx, nil, toolsy.ToolInput{ArgsJSON: []byte(`{}`)}, func(toolsy.Chunk) error { return nil })
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dispatch never started")
	}
	// Act: reject excess work before SDK; cancel the one admitted invocation.
	_, admission := execute(t, snapshot.Tools[0], `{}`)
	cancel()
	// Assert.
	require.ErrorIs(t, admission, ErrAdmission)
	select {
	case callErr := <-result:
		require.ErrorIs(t, callErr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller remained blocked")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("server child remained live")
	}
	_, err = session.ListTools(t.Context(), nil)
	require.NoError(t, err)
}

func TestSDKConsumerAbortAndLateProgress(t *testing.T) {
	// Arrange: host handler waits for progress delivery before emitting terminal.
	seen := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	server := sdk.NewServer(&sdk.Implementation{Name: "abort", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			if err := req.Session.NotifyProgress(
				ctx,
				&sdk.ProgressNotificationParams{ProgressToken: req.Params.GetProgressToken(), Progress: 1},
			); err != nil {
				return nil, err
			}
			select {
			case <-seen:
			case <-ctx.Done():
			}
			<-ctx.Done()
			finished <- struct{}{}
			return nil, ctx.Err()
		})
	importer, session := adapterPair(t, server, false, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	cause := errors.New("consumer stopped")
	// Act: consumer rejects progress. Only its SDK child is cancelled.
	err = snapshot.Tools[0].Execute(
		t.Context(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(chunk toolsy.Chunk) error {
			if chunk.Event == toolsy.EventProgress {
				seen <- struct{}{}
				return cause
			}
			return nil
		},
	)
	// Assert.
	require.ErrorIs(t, err, cause)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("consumer abort did not cancel dispatch")
	}
	_, err = session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	iCalls := func() int { importer.mu.Lock(); defer importer.mu.Unlock(); return len(importer.calls) }
	require.Eventually(t, func() bool { return iCalls() == 0 }, time.Second, time.Millisecond)
	// Act / Assert: a retired token cannot allocate new work.
	importer.progress(&sdk.ProgressNotificationParams{ProgressToken: "toolsy-1", Progress: 99})
	require.Zero(t, iCalls())
}

func TestSDKInvalidationCallbackCompositionAndDisconnect(t *testing.T) {
	// Arrange: callbacks installed before Connect, unchanged host callback retained.
	var calls atomic.Int32
	var notifications atomic.Int32
	server := echoServer(&calls, "")
	importer, session := adapterPair(t, server, false, importConfig(), &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) { notifications.Add(1) },
	})
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act: replace the remote descriptor; SDK delivers the notification.
	server.AddTool(&sdk.Tool{Name: "echo", Description: "replacement", InputSchema: json.RawMessage(objectSchema)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	// Assert.
	select {
	case update := <-importer.Updates():
		require.Equal(t, "tools", update.Kind)
	case <-time.After(time.Second):
		t.Fatal("no invalidation")
	}
	require.Eventually(t, func() bool { return notifications.Load() >= 1 }, time.Second, time.Millisecond)
	_, err = execute(t, snapshot.Tools[0], `{"n":1}`)
	require.ErrorIs(t, err, ErrStale)
	fresh, err := importer.Discover(t.Context())
	require.NoError(t, err)
	require.Equal(t, "replacement", fresh.Descriptors[0].Description)
	// Act / Assert: host closes its SDK session; adapter's listeners terminate.
	require.NoError(t, session.Close())
	select {
	case <-importer.Done():
	case <-time.After(time.Second):
		t.Fatal("adapter did not observe disconnect")
	}
}

func TestSDKTwoSessionsOneClientIsolation(t *testing.T) {
	// Arrange: one SDK client, independent host authority bindings.
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "shared", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	connect := func(name string) (*Importer, *sdk.ClientSession) {
		var calls atomic.Int32
		server := echoServer(&calls, "")
		a, b := sdk.NewInMemoryTransports()
		ss, serverErr := server.Connect(t.Context(), b, nil)
		require.NoError(t, serverErr)
		t.Cleanup(func() { _ = ss.Close() })
		cs, clientErr := client.Connect(t.Context(), a, nil)
		require.NoError(t, clientErr)
		t.Cleanup(func() { _ = cs.Close() })
		importer, bindErr := hooks.Bind(cs, name)
		require.NoError(t, bindErr)
		t.Cleanup(func() { _ = importer.Close() })
		return importer, cs
	}
	a, _ := connect("tenant-a")
	b, bs := connect("tenant-b")
	first, err := a.Discover(t.Context())
	require.NoError(t, err)
	second, err := b.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	a.invalidate("tools", "")
	_, revoked := execute(t, first.Tools[0], `{"n":1}`)
	_, valid := execute(t, second.Tools[0], `{"n":2}`)
	require.NoError(t, a.Close())
	_, stillLive := bs.ListTools(t.Context(), nil)
	// Assert: equal SDK DTOs never produce cross-authority digest or revocation.
	require.NotEqual(t, first.Digest, second.Digest)
	require.ErrorIs(t, revoked, ErrStale)
	require.NoError(t, valid)
	require.NoError(t, stillLive)
}

func TestSDKSoftErrorAndFaultAreDistinct(t *testing.T) {
	// Arrange.
	server := sdk.NewServer(&sdk.Implementation{Name: "errors", Version: "1"}, nil)
	for _, name := range []string{"soft", "fault"} {
		server.AddTool(&sdk.Tool{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				if req.Params.Name == "fault" {
					return nil, errors.New("server-secret")
				}
				return &sdk.CallToolResult{
					IsError: true,
					Content: []sdk.Content{&sdk.TextContent{Text: "business failed"}},
				}, nil
			})
	}
	importer, _ := adapterPair(t, server, true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	var soft, fault toolsy.Tool
	for _, tool := range snapshot.Tools {
		if tool.Manifest().Name == "soft" {
			soft = tool
		} else {
			fault = tool
		}
	}
	// Act.
	chunks, softErr := execute(t, soft, `{}`)
	_, faultErr := execute(t, fault, `{}`)
	// Assert.
	require.NoError(t, softErr)
	require.True(t, chunks[0].IsError)
	require.Equal(t, toolsy.CodeRemoteExecution, chunks[0].ToolEnvelope().Error.Code)
	var bridge *BridgeError
	require.ErrorAs(t, faultErr, &bridge)
	require.NotContains(t, bridge.Error(), "server-secret")
}

type tenantContextKey struct{}

func TestSDKExportCrossScopeDiscoveryAndDirectCall(t *testing.T) {
	// Arrange: the host authenticates context and chooses a server per scope.
	servers := map[string]*sdk.Server{}
	var dispatch atomic.Int32
	for _, tenant := range []string{"a", "b"} {
		tool, err := toolsy.NewProxyTool("tool_"+tenant, "scoped", []byte(`{"type":"object"}`),
			func(_ context.Context, env *toolsy.RunEnv, _ []byte, yield func(toolsy.Chunk) error) error {
				require.Equal(t, tenant, env.CallContext().Scope)
				dispatch.Add(1)
				return yield(
					toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"ok"`), MimeType: toolsy.MimeTypeJSON},
				)
			})
		require.NoError(t, err)
		registry, err := toolsy.NewRegistry(tool)
		require.NoError(t, err)
		view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"tool_" + tenant}, Owner: tenant})
		require.NoError(t, err)
		servers[tenant], err = Export(
			view,
			ExportConfig{
				Names:          []string{"tool_" + tenant},
				Implementation: &sdk.Implementation{Name: tenant, Version: "1"},
				Resolve: func(ctx context.Context, _ sdk.Request) (toolsy.CallContext, error) {
					if ctx.Value(tenantContextKey{}) != tenant {
						return toolsy.CallContext{}, errors.New("wrong scope")
					}
					return toolsy.NewCallContext("host-user", tenant), nil
				},
			},
		)
		require.NoError(t, err)
	}
	handler := sdk.NewStreamableHTTPHandler(
		func(r *http.Request) *sdk.Server { return servers[r.Context().Value(tenantContextKey{}).(string)] },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test credentials are mapped to host identity before SDK, never by arguments.
		tenant := r.Header.Get("Authorization")
		if servers[tenant] == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantContextKey{}, tenant)))
	}))
	t.Cleanup(host.Close)
	for _, tenant := range []string{"a", "b"} {
		client := sdk.NewClient(
			&sdk.Implementation{Name: tenant, Version: "1"},
			&sdk.ClientOptions{MultiRoundTrip: &sdk.MultiRoundTripOptions{Disabled: true}},
		)
		httpClient := host.Client()
		transport := httpClient.Transport
		auth := &http.Client{Transport: authTransport{base: transport, token: tenant}}
		session, err := client.Connect(
			t.Context(),
			&sdk.StreamableClientTransport{Endpoint: host.URL, HTTPClient: auth},
			nil,
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = session.Close() })
		// Act.
		list, err := session.ListTools(t.Context(), nil)
		require.NoError(t, err)
		_, hidden := session.CallTool(
			t.Context(),
			&sdk.CallToolParams{Name: "tool_" + map[string]string{"a": "b", "b": "a"}[tenant]},
		)
		_, own := session.CallTool(
			t.Context(),
			&sdk.CallToolParams{Name: "tool_" + tenant, Arguments: map[string]any{}, Meta: sdk.Meta{"scope": "other"}},
		)
		// Assert.
		require.Len(t, list.Tools, 1)
		require.Equal(t, "tool_"+tenant, list.Tools[0].Name)
		require.Error(t, hidden)
		require.NoError(t, own)
	}
	require.EqualValues(t, 2, dispatch.Load())
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	owned := r.Clone(r.Context())
	owned.Header.Set("Authorization", a.token)
	return a.base.RoundTrip(owned)
}
