package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestSDKSchemaDefaultNullZeroAndDialectParity(t *testing.T) {
	for _, dialect := range []string{"http://json-schema.org/draft-07/schema#", "https://json-schema.org/draft/2020-12/schema"} {
		t.Run(dialect, func(t *testing.T) {
			// Arrange: explicit schemas, a nullable value and a default which is not input.
			schema := map[string]any{
				"$schema": dialect,
				"type":    "object",
				"properties": map[string]any{
					"n":         map[string]any{"type": "integer"},
					"nullable":  map[string]any{"type": []string{"null", "string"}},
					"defaulted": map[string]any{"type": "string", "default": "host-visible-default"},
				},
				"required":             []string{"n"},
				"additionalProperties": false,
			}
			server := sdk.NewServer(&sdk.Implementation{Name: "schema", Version: "1"}, nil)
			input := make(chan json.RawMessage, 1)
			server.AddTool(
				&sdk.Tool{Name: "schema", InputSchema: schema},
				func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
					input <- req.Params.Arguments
					return &sdk.CallToolResult{StructuredContent: req.Params.Arguments}, nil
				},
			)
			importer, _ := adapterPair(t, server, true, importConfig(), nil)
			snapshot, err := importer.Discover(t.Context())
			require.NoError(t, err)
			// Act: zero and null remain actual arguments, absent default remains absent.
			chunks, err := execute(t, snapshot.Tools[0], `{"n":0,"nullable":null}`)
			// Assert: descriptor/manifest parity and no default mutation.
			require.NoError(t, err)
			require.JSONEq(t, `{"n":0,"nullable":null}`, string(<-input))
			require.JSONEq(t, `{"n":0,"nullable":null}`, string(chunks[0].Data))
			source, err := json.Marshal(snapshot.Descriptors[0].InputSchema)
			require.NoError(t, err)
			manifest, err := json.Marshal(snapshot.Tools[0].Manifest().Parameters)
			require.NoError(t, err)
			require.JSONEq(t, string(source), string(manifest))
			_, err = execute(t, snapshot.Tools[0], `{"nullable":null}`)
			require.Error(t, err)
			require.Empty(t, input)
		})
	}
}

func TestSDKInvalidationDuringCallRevokesSuccessfulDelivery(t *testing.T) {
	// Arrange: hold an already dispatched remote effect while the catalog changes.
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := sdk.NewServer(&sdk.Implementation{Name: "inflight", Version: "1"}, nil)
	tool := &sdk.Tool{Name: "one", InputSchema: json.RawMessage(`{"type":"object"}`)}
	handler := func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		calls.Add(1)
		close(entered)
		<-release
		return &sdk.CallToolResult{StructuredContent: json.RawMessage(`{"ok":true}`)}, nil
	}
	server.AddTool(tool, handler)
	importer, _ := adapterPair(t, server, false, importConfig(), nil)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	result := make(chan error, 1)
	var delivered atomic.Int32
	// Act: SDK notification revokes the proxy before its result reaches the core consumer.
	go func() {
		result <- snapshot.Tools[0].Execute(t.Context(), nil, toolsy.ToolInput{ArgsJSON: []byte(`{}`)}, func(toolsy.Chunk) error { delivered.Add(1); return nil })
	}()
	<-entered
	server.AddTool(&sdk.Tool{Name: "one", Description: "changed", InputSchema: tool.InputSchema}, handler)
	select {
	case <-importer.Updates():
	case <-time.After(time.Second):
		t.Fatal("invalidation missing")
	}
	close(release)
	// Assert: no success delivered, no repeat effect, stale use rejected.
	require.ErrorIs(t, <-result, ErrStale)
	require.Zero(t, delivered.Load())
	require.EqualValues(t, 1, calls.Load())
	_, err = execute(t, snapshot.Tools[0], `{}`)
	require.ErrorIs(t, err, ErrStale)
	require.EqualValues(t, 1, calls.Load())
}

func TestSDKConcurrentProgressSerializedAndDrained(t *testing.T) {
	// Arrange: host callbacks acknowledge acceptance before each server terminal.
	acknowledged := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	server := sdk.NewServer(&sdk.Implementation{Name: "concurrent", Version: "1"}, nil)
	server.AddTool(
		&sdk.Tool{Name: "progress", InputSchema: json.RawMessage(objectSchema)},
		func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			var args struct {
				N int `json:"n"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			if err := req.Session.NotifyProgress(
				ctx,
				&sdk.ProgressNotificationParams{
					ProgressToken: req.Params.GetProgressToken(),
					Progress:      1,
					Message:       strconv.Itoa(args.N),
				},
			); err != nil {
				return nil, err
			}
			select {
			case <-acknowledged[args.N]:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &sdk.CallToolResult{StructuredContent: req.Params.Arguments}, nil
		},
	)
	importer, _ := adapterPair(
		t,
		server,
		false,
		importConfig(),
		&sdk.ClientOptions{
			ProgressNotificationHandler: func(_ context.Context, req *sdk.ProgressNotificationClientRequest) {
				n, err := strconv.Atoi(req.Params.Message)
				if err == nil {
					close(acknowledged[n])
				}
			},
		},
	)
	snapshot, err := importer.Discover(t.Context())
	require.NoError(t, err)
	var wg sync.WaitGroup
	var mu sync.Mutex
	tokens := map[string]bool{}
	// Act: concurrent calls have distinct payloads and serialized per-call consumers.
	for n := range 4 {
		wg.Go(func() { checkProgressCall(t, snapshot.Tools[0], n, &mu, tokens) })
	}
	wg.Wait()
	// Assert: no cross-call token collision.
	require.Len(t, tokens, 4)
}

func checkProgressCall(t *testing.T, tool toolsy.Tool, n int, mu *sync.Mutex, tokens map[string]bool) {
	t.Helper()
	var active atomic.Int32
	var progress int
	var terminal bool
	err := tool.Execute(
		t.Context(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{"n":` + strconv.Itoa(n) + `}`)},
		func(chunk toolsy.Chunk) error {
			require.EqualValues(t, 1, active.Add(1))
			defer active.Add(-1)
			switch chunk.Event {
			case toolsy.EventProgress:
				require.False(t, terminal)
				require.Equal(t, strconv.Itoa(n), chunk.Progress.Message)
				progress++
				mu.Lock()
				require.False(t, tokens[chunk.Progress.Token])
				tokens[chunk.Progress.Token] = true
				mu.Unlock()
				time.Sleep(time.Millisecond)
			case toolsy.EventControl:
				t.Fatal("unexpected control event")
			case toolsy.EventResult:
				terminal = true
				require.Equal(t, 1, progress)
				require.JSONEq(t, `{"n":`+strconv.Itoa(n)+`}`, string(chunk.Data))
			}
			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, terminal)
}

func TestSDKExportOutcomeProjectionAndPostEffectFailure(t *testing.T) {
	for _, mode := range []string{"empty", "noop", "soft", "fault", "limit", "projection"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: every dispatch records exactly one effect before delivery/projection.
			var calls atomic.Int32
			local, err := toolsy.NewProxyTool(
				"outcome",
				"outcome",
				[]byte(`{"type":"object"}`),
				func(_ context.Context, _ *toolsy.RunEnv, _ []byte, yield func(toolsy.Chunk) error) error {
					calls.Add(1)
					switch mode {
					case "empty":
						return yield(toolsy.Chunk{Event: toolsy.EventResult, EmptyResult: true})
					case "noop":
						return yield(toolsy.Chunk{Event: toolsy.EventResult, Noop: true})
					case "soft":
						return yield(
							toolsy.NewErrorChunkFromErr(
								&toolsy.ToolError{Code: toolsy.CodeRemoteExecution, Reason: "private business failure"},
							),
						)
					case "fault":
						return errors.New("private fault")
					}
					return yield(
						toolsy.Chunk{
							Event:    toolsy.EventResult,
							Data:     []byte(`{"value":"long result"}`),
							MimeType: toolsy.MimeTypeJSON,
							Effects:  []any{"internal-effect"},
						},
					)
				},
			)
			require.NoError(t, err)
			registry, err := toolsy.NewRegistry(local)
			require.NoError(t, err)
			view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{"outcome"}})
			require.NoError(t, err)
			config := ExportConfig{
				Names:          []string{"outcome"},
				Implementation: &sdk.Implementation{Name: "outcomes", Version: "1"},
				Resolve: func(context.Context, sdk.Request) (toolsy.CallContext, error) {
					return toolsy.NewCallContext("host", "scope"), nil
				},
			}
			if mode == "limit" {
				config.MaxResultBytes = 1
			}
			if mode == "projection" {
				config.Project = func(context.Context, toolsy.ToolManifest, []toolsy.Chunk) (*sdk.CallToolResult, error) {
					return nil, errors.New("private projection failure")
				}
			}
			server, err := Export(view, config)
			require.NoError(t, err)
			_, session := adapterPair(t, server, true, importConfig(), nil)
			// Act.
			result, callErr := session.CallTool(
				t.Context(),
				&sdk.CallToolParams{Name: "outcome", Arguments: json.RawMessage(`{}`)},
			)
			// Assert: empty/soft outcomes distinct; post-effect failure never repairs/resends.
			require.EqualValues(t, 1, calls.Load())
			switch mode {
			case "empty", "noop":
				require.NoError(t, callErr)
				require.Empty(t, result.Content)
				require.Nil(t, result.StructuredContent)
				require.False(t, result.IsError)
			case "soft":
				require.NoError(t, callErr)
				require.True(t, result.IsError)
				raw, encodeErr := json.Marshal(result)
				require.NoError(t, encodeErr)
				require.NotContains(t, string(raw), "private")
			default:
				require.Error(t, callErr)
				require.NotContains(t, callErr.Error(), "private")
			}
		})
	}
}
