package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

const objectSchema = `{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"],"additionalProperties":false}`

func importConfig() ImportConfig {
	return ImportConfig{Classify: func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) {
		return []toolsy.ToolOption{toolsy.WithDangerous()}, nil
	}}
}

func adapterPair(
	t *testing.T,
	server *sdk.Server,
	httpWire bool,
	config ImportConfig,
	options *sdk.ClientOptions,
) (*Importer, *sdk.ClientSession) {
	t.Helper()
	hooks, composed, err := ComposeClient(options, config)
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, composed)
	require.NoError(t, hooks.Install(client))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	var transport sdk.Transport
	if httpWire {
		host := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
			&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
		t.Cleanup(host.Close)
		transport = &sdk.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client()}
	} else {
		a, b := sdk.NewInMemoryTransports()
		session, connectErr := server.Connect(ctx, b, nil)
		require.NoError(t, connectErr)
		t.Cleanup(func() { _ = session.Close() })
		transport = a
	}
	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	importer, err := hooks.Bind(session, "host-scope/test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = importer.Close() })
	return importer, session
}

func echoServer(calls *atomic.Int32, output string) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "echo", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "echo", InputSchema: json.RawMessage(objectSchema),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`)},
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			calls.Add(1)
			value := req.Params.Arguments
			if output != "" {
				value = json.RawMessage(output)
			}
			return &sdk.CallToolResult{StructuredContent: value}, nil
		})
	return server
}

func execute(t *testing.T, tool toolsy.Tool, raw string) ([]toolsy.Chunk, error) {
	t.Helper()
	registry, err := toolsy.NewRegistry(tool)
	require.NoError(t, err)
	var chunks []toolsy.Chunk
	err = registry.Execute(t.Context(), toolsy.ToolCall{ToolName: tool.Manifest().Name,
		Input: toolsy.ToolInput{CallID: "app-call-not-progress", ArgsJSON: []byte(raw)}},
		func(chunk toolsy.Chunk) error { chunks = append(chunks, chunk); return nil })
	return chunks, err
}

func TestSDKImportSchemaAndOwnership(t *testing.T) {
	for _, wire := range []bool{false, true} {
		t.Run(strconv.FormatBool(wire), func(t *testing.T) {
			// Arrange: real SDK client/server; snapshots are caller-owned.
			var calls atomic.Int32
			importer, session := adapterPair(t, echoServer(&calls, ""), wire, importConfig(), nil)
			snapshot, err := importer.Discover(t.Context())
			require.NoError(t, err)
			snapshot.Descriptors[0].Name = "forged"
			snapshot.Descriptors[0].InputSchema.(map[string]any)["type"] = "array"
			// Act.
			chunks, valid := execute(t, snapshot.Tools[0], `{"n":7}`)
			_, invalid := execute(t, snapshot.Tools[0], `{"n":"bad"}`)
			cached, listErr := session.ListTools(t.Context(), nil)
			// Assert: mutations never change SDK state or a compiled proxy contract.
			require.NoError(t, valid)
			require.Error(t, invalid)
			require.EqualValues(t, 1, calls.Load())
			require.Len(t, chunks, 1)
			require.JSONEq(t, `{"n":7}`, string(chunks[0].Data))
			require.IsType(t, &sdk.CallToolResult{}, chunks[0].TypedResult)
			require.NoError(t, listErr)
			require.Equal(t, "echo", cached.Tools[0].Name)
			require.Len(t, snapshot.Digest, 64)
		})
	}
}

func TestSDKBadOutputIsNotRetried(t *testing.T) {
	// Arrange: server may have effected before returning a schema-invalid result.
	var calls atomic.Int32
	importer, _ := adapterPair(t, echoServer(&calls, `{"n":"invalid"}`), true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	chunks, err := execute(t, snapshot.Tools[0], `{"n":7}`)
	// Assert.
	var contract *toolsy.ResultContractError
	require.ErrorAs(t, err, &contract)
	require.Empty(t, chunks)
	require.EqualValues(t, 1, calls.Load())
}

func TestSDKMRTRManualRoundAndParamsOwnership(t *testing.T) {
	// Arrange: request state is opaque; continuation is supplied by the host context.
	var calls atomic.Int32
	server := sdk.NewServer(&sdk.Implementation{Name: "mrtr", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "round", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			if calls.Add(1) == 1 {
				return &sdk.CallToolResult{InputRequests: sdk.InputRequestMap{}, RequestState: "opaque-✓"}, nil
			}
			require.Equal(t, "opaque-✓", req.Params.RequestState)
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "done"}}}, nil
		})
	importer, _ := adapterPair(t, server, true, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	_, err = execute(t, snapshot.Tools[0], `{}`)
	// Assert.
	var needs *NeedsInputError
	require.ErrorAs(t, err, &needs)
	require.EqualValues(t, 1, calls.Load())
	ctx, err := WithRound(t.Context(), Round{RequestState: needs.Result.(*sdk.CallToolResult).RequestState})
	require.NoError(t, err)
	// Act: explicit second attempt through the same prepared tool.
	err = snapshot.Tools[0].Execute(
		ctx,
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert.
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
}

func TestSDKImportCurrentPolicyAndReplay(t *testing.T) {
	// Arrange: cached results never waive the current authorization gate.
	var calls atomic.Int32
	importer, _ := adapterPair(t, echoServer(&calls, ""), false, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	cache, err := toolsy.NewResultCache(toolsy.NewMemoryResultCacheStore(),
		func(context.Context, toolsy.PreparedCall) (bool, error) { return true, nil },
		func(context.Context, toolsy.PreparedCall) (string, error) { return "host-tenant", nil },
		toolsy.JSONResultCodec[*sdk.CallToolResult, string]{}, 0)
	require.NoError(t, err)
	denied := false
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(cache), toolsy.WithPolicy("current",
		toolsy.PolicyFunc(func(context.Context, toolsy.PolicyRequest) toolsy.Decision {
			if denied {
				return toolsy.DenyDecision("revoked")
			}
			return toolsy.AllowDecision()
		}))).Add(snapshot.Tools...).Build()
	require.NoError(t, err)
	call := toolsy.ToolCall{ToolName: "echo", Input: toolsy.ToolInput{ArgsJSON: []byte(`{"n":1}`)}}
	yield := func(toolsy.Chunk) error { return nil }
	// Act.
	require.NoError(t, registry.Execute(t.Context(), call, yield))
	require.NoError(t, registry.Execute(t.Context(), call, yield))
	denied = true
	err = registry.Execute(t.Context(), call, yield)
	// Assert.
	require.ErrorIs(t, err, toolsy.ErrPolicyDenied)
	require.EqualValues(t, 1, calls.Load())
	// Act: discovery replacement revokes old proxy including cached replay.
	denied = false
	_, err = importer.Discover(t.Context())
	require.NoError(t, err)
	err = registry.Execute(t.Context(), call, yield)
	// Assert.
	require.ErrorIs(t, err, ErrStale)
	require.EqualValues(t, 1, calls.Load())
}

func TestSDKProgressCompositionIsolationAndConsumerAbort(t *testing.T) {
	// Arrange: callbacks installed before Connect; each call owns its token.
	var hostProgress atomic.Int32
	server := sdk.NewServer(&sdk.Implementation{Name: "progress", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "progress", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			err := req.Session.NotifyProgress(
				ctx,
				&sdk.ProgressNotificationParams{
					ProgressToken: req.Params.GetProgressToken(),
					Progress:      1,
					Message:       "ready",
				},
			)
			if err != nil {
				return nil, err
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "done"}}}, nil
		})
	importer, session := adapterPair(t, server, false, importConfig(), &sdk.ClientOptions{
		ProgressNotificationHandler: func(context.Context, *sdk.ProgressNotificationClientRequest) { hostProgress.Add(1) },
	})
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			// Act.
			_, executeErr := execute(t, snapshot.Tools[0], `{}`)
			// Assert.
			require.NoError(t, executeErr)
		})
	}
	wg.Wait()
	require.Eventually(t, func() bool { return hostProgress.Load() == 4 }, time.Second, time.Millisecond)
	// Act: adapter close revokes its tools but leaves the borrowed connection live.
	require.NoError(t, importer.Close())
	_, err = session.ListTools(t.Context(), nil)
	// Assert.
	require.NoError(t, err)
	_, err = execute(t, snapshot.Tools[0], `{}`)
	require.Error(t, err)
}

func TestSDKProgressQueueAndRetirement(t *testing.T) {
	// Arrange: adapter callback queue is finite and single-consumer.
	ctx, cancel := context.WithCancel(t.Context())
	i := &Importer{calls: map[string]*progressCall{}, config: ImportConfig{ProgressBuffer: 1}}
	state := &progressCall{queue: make(chan toolsy.Chunk, 1), cancel: cancel}
	i.calls["opaque"] = state
	// Act.
	i.progress(&sdk.ProgressNotificationParams{ProgressToken: "opaque", Progress: 1})
	i.progress(&sdk.ProgressNotificationParams{ProgressToken: "opaque", Progress: 2})
	i.retire("opaque")
	i.progress(&sdk.ProgressNotificationParams{ProgressToken: "opaque", Progress: 3})
	// Assert.
	require.ErrorIs(t, state.err, ErrQueueFull)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Len(t, state.queue, 1)
}

func scopedExport(t *testing.T, calls *atomic.Int32, denied *atomic.Bool) *sdk.Server {
	t.Helper()
	tool, err := toolsy.NewProxyTool("local", "local", []byte(objectSchema),
		func(_ context.Context, _ *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			calls.Add(1)
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args, MimeType: toolsy.MimeTypeJSON,
				Effects: []any{"private-effect"}})
		}, toolsy.WithOutputSchema(map[string]any{"type": "object"}))
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithPolicy("current", toolsy.PolicyFunc(func(context.Context, toolsy.PolicyRequest) toolsy.Decision {
		if denied.Load() {
			return toolsy.DenyDecision("revoked")
		}
		return toolsy.AllowDecision()
	}))).
		Add(tool).
		Build()
	require.NoError(t, err)
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"local"}, Owner: "tenant-a"})
	require.NoError(t, err)
	server, err := Export(
		view,
		ExportConfig{Names: []string{"local"}, Implementation: &sdk.Implementation{Name: "export", Version: "1"},
			Resolve: func(context.Context, sdk.Request) (toolsy.CallContext, error) {
				return toolsy.NewCallContext("authenticated-user", "tenant-a"), nil
			}},
	)
	require.NoError(t, err)
	return server
}

func TestSDKExportScopedPolicyAndProjection(t *testing.T) {
	for _, wire := range []bool{false, true} {
		t.Run(strconv.FormatBool(wire), func(t *testing.T) {
			// Arrange: production export invokes the scoped prepared executor.
			var calls atomic.Int32
			var denied atomic.Bool
			_, session := adapterPair(t, scopedExport(t, &calls, &denied), wire, importConfig(), nil)
			// Act.
			list, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)
			result, err := session.CallTool(
				t.Context(),
				&sdk.CallToolParams{
					Name:      "local",
					Arguments: json.RawMessage(`{"n":7}`),
					Meta:      sdk.Meta{"scope": "attacker", "grant": "forged"},
				},
			)
			// Assert.
			require.NoError(t, err)
			require.Len(t, list.Tools, 1)
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "private-effect")
			require.NotContains(t, string(raw), "forged")
			require.Equal(t, map[string]any{"n": json.Number("7")}, result.StructuredContent)
			// Act: hidden direct call, invalid input and current revoke.
			_, hidden := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "hidden"})
			_, invalid := session.CallTool(
				t.Context(),
				&sdk.CallToolParams{Name: "local", Arguments: map[string]any{"n": "bad"}},
			)
			denied.Store(true)
			_, revoked := session.CallTool(
				t.Context(),
				&sdk.CallToolParams{Name: "local", Arguments: map[string]any{"n": 7}},
			)
			// Assert.
			require.Error(t, hidden)
			require.Error(t, invalid)
			require.Error(t, revoked)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestSDKDiagnosticCausesStayInspectable(t *testing.T) {
	// Arrange.
	secret := errors.New("credential-secret")
	err := &BridgeError{Kind: "SDK call", Cause: secret}
	// Act / Assert.
	require.ErrorIs(t, err, secret)
	for _, verb := range []string{"%s", "%v", "%+v", "%#v"} {
		require.NotContains(t, fmt.Sprintf(verb, err), "credential-secret")
	}
}

func TestSDKStdioHelper(t *testing.T) {
	if os.Getenv("TOOLSY_SDK_HELPER") != "1" {
		return
	}
	// Arrange: subprocess uses production export; stdout belongs only to SDK.
	var calls atomic.Int32
	var denied atomic.Bool
	server := scopedExport(t, &calls, &denied)
	// Act.
	err := server.Run(context.Background(), &sdk.StdioTransport{})
	// Assert.
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestSDKSTDIOImportAndExport(t *testing.T) {
	// Arrange: real subprocess, official transports in both directions.
	hooks, options, err := ComposeClient(nil, importConfig())
	require.NoError(t, err)
	client := sdk.NewClient(&sdk.Implementation{Name: "stdio", Version: "1"}, options)
	require.NoError(t, hooks.Install(client))
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSDKStdioHelper$")
	command.Env = append(os.Environ(), "TOOLSY_SDK_HELPER=1")
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	importer, err := hooks.Bind(session, "stdio/scope")
	require.NoError(t, err)
	t.Cleanup(func() { _ = importer.Close() })
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	// Act.
	chunks, err := execute(t, snapshot.Tools[0], `{"n":9}`)
	// Assert.
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.JSONEq(t, `{"n":9}`, string(chunks[0].Data))
	require.NoError(t, importer.Close())
	require.NoError(t, session.Close())
	require.NotNil(t, command.ProcessState)
}
