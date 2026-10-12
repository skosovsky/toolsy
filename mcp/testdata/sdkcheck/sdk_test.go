package sdkcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const exactInteger = "9007199254740993"
const roundedInteger = "9007199254740992"
const testTimeout = 10 * time.Second

func newClient(disabled bool) *mcp.Client {
	return mcp.NewClient(&mcp.Implementation{Name: "sdkcheck", Version: "1"}, &mcp.ClientOptions{
		Capabilities:   &mcp.ClientCapabilities{},
		MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: disabled},
	})
}

func connect(t *testing.T, server *mcp.Server, client *mcp.Client, httpWire bool) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	t.Cleanup(cancel)
	var transport mcp.Transport
	if httpWire {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
		host := httptest.NewServer(handler)
		t.Cleanup(host.Close)
		transport = &mcp.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client()}
	} else {
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		session, err := server.Connect(ctx, serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		transport = clientTransport
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func object(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func idRoundTrip(t *testing.T, id string) string {
	t.Helper()
	message, err := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":` + id + `,"method":"probe"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jsonrpc.EncodeMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	return string(object(t, raw)["id"])
}

func TestCharacterizationJSONRPCIDs(t *testing.T) {
	// Arrange: public SDK codec, including a string ID and exponent spelling.
	for _, fixture := range []struct{ name, input, observed string }{
		{"integer", exactInteger, exactInteger},
		{"string", `"9007199254740993"`, `"9007199254740993"`},
		{"exponent", "9.007199254740993e15", exactInteger},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			got := idRoundTrip(t, fixture.input)
			// Assert: observed upstream behavior, NOT migration acceptance.
			if got != fixture.observed {
				t.Fatalf("SDK behavior changed; reassess blocker: input=%s got=%s", fixture.input, got)
			}
			t.Logf("input=%s output=%s", fixture.input, got)
		})
	}
}

func fidelityResult(t *testing.T, httpWire bool, value string) *mcp.CallToolResult {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "fidelity", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: json.RawMessage(value),
				Meta: mcp.Meta{"vendor.example/payload": json.RawMessage(`{"id":` + exactInteger + `}`)}}, nil
		})
	session := connect(t, server, newClient(true), httpWire)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "probe"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCharacterizationStructuredContentActualWire(t *testing.T) {
	// Arrange: raw exact JSON emitted by an official SDK server.
	for _, wire := range []struct {
		name string
		http bool
	}{{"inmemory", false}, {"HTTP", true}} {
		t.Run(wire.name, func(t *testing.T) {
			for _, fixture := range []struct{ name, input, observed string }{
				{"object", `{"id":` + exactInteger + `}`, `{"id":` + exactInteger + `}`},
				{"array", `[` + exactInteger + `]`, `[` + exactInteger + `]`},
				{"scalar", exactInteger, exactInteger},
				{"exponent", "9.007199254740993e15", "9.007199254740993e15"},
				{"null", "null", "null"},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					// Act.
					result := fidelityResult(t, wire.http, fixture.input)
					value := string(marshal(t, result.StructuredContent))
					metadata := object(t, marshal(t, result.Meta["vendor.example/payload"]))
					// Assert: structured content preserves numbers; metadata still rounds.
					if value != fixture.observed || string(metadata["id"]) != roundedInteger {
						t.Fatalf("reassess SDK behavior: value=%s metadata=%s", value, metadata["id"])
					}
					t.Logf("input=%s output=%s meta.id=%s", fixture.input, value, metadata["id"])
				})
			}
		})
	}
}

func TestCharacterizationSchemaActualWire(t *testing.T) {
	// Arrange: constraint must distinguish adjacent integers above float64 precision.
	server := mcp.NewServer(&mcp.Implementation{Name: "schema", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "probe", InputSchema: json.RawMessage(
		`{"type":"object","properties":{"id":{"type":"integer","minimum":` + exactInteger + `}}}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	session := connect(t, server, newClient(true), true)
	// Act.
	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	schema := object(t, marshal(t, result.Tools[0].InputSchema))
	properties := object(t, schema["properties"])
	constraint := object(t, properties["id"])
	// Assert: schema semantics change on the actual HTTP list path.
	if string(constraint["minimum"]) != roundedInteger {
		t.Fatalf("reassess SDK behavior: minimum=%s", constraint["minimum"])
	}
	t.Logf("minimum=%s -> %s", exactInteger, constraint["minimum"])
}

func TestCharacterizationDTORepresentationLoss(t *testing.T) {
	// Arrange: directly characterize DTOs separately from connection validation.
	for _, raw := range []string{
		`{"resultType":"complete","content":[],"structuredContent":null,"vendorField":"keep"}`,
		`{"resultType":"complete","content":[],"vendorField":"keep"}`,
	} {
		var result mcp.CallToolResult
		// Act.
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatal(err)
		}
		fields := object(t, marshal(t, &result))
		// Assert: explicit null survives; unrecognized top-level fields vanish.
		_, expectedNull := object(t, []byte(raw))["structuredContent"]
		value, present := fields["structuredContent"]
		if present != expectedNull || (present && string(value) != "null") {
			t.Fatal("reassess null handling")
		}
		if _, present := fields["vendorField"]; present {
			t.Fatal("reassess extension handling")
		}
	}
}

func TestCharacterizationPermissiveDTO(t *testing.T) {
	// Arrange: this does not assert the same acceptance on a complete connection path.
	for _, raw := range []string{
		`{"content":[]}`,
		`{"resultType":"nonsense","content":[]}`,
		`{"resultType":"complete","content":[],"structuredContent":{"a":1,"a":2}}`,
	} {
		var result mcp.CallToolResult
		// Act.
		err := json.Unmarshal([]byte(raw), &result)
		// Assert.
		if err != nil {
			t.Fatalf("reassess decoder behavior: %v", err)
		}
	}
}

func TestCharacterizationCatalogCacheAliasing(t *testing.T) {
	// Arrange: a private TTL cache and a counted network list handler.
	var reads atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "cache", Version: "1"}, &mcp.ServerOptions{
		SetCacheable: func(_ context.Context, _ mcp.Request, cache *mcp.Cacheable) {
			cache.TTLMs = 60000
			cache.CacheScope = "private"
		},
	})
	server.AddTool(&mcp.Tool{Name: "probe", Description: "original", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				reads.Add(1)
			}
			return next(ctx, method, req)
		}
	})
	session := connect(t, server, newClient(true), false)
	first, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Act: mutation is deliberate adversarial caller behavior.
	first.Tools[0].Description = "caller mutation"
	second, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: adaptation must copy BEFORE sharing, not mutate SDK cache hints.
	if reads.Load() != 1 || first != second || second.Tools[0].Description != "caller mutation" {
		t.Fatalf("reassess cache ownership: reads=%d same=%v", reads.Load(), first == second)
	}
}

func TestMRTRSingleAttemptAndManualContinuation(t *testing.T) {
	// Arrange: an opaque state including non-ASCII, and automatic/manual modes.
	for _, fixture := range []struct {
		name     string
		disabled bool
		calls    int32
	}{
		{"automatic", false, 2}, {"manual", true, 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			checkMRTR(t, fixture.disabled, fixture.calls)
		})
	}
}

func checkMRTR(t *testing.T, disabled bool, wantCalls int32) {
	t.Helper()
	// Arrange.
	var calls atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "mrtr", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if calls.Add(1) == 1 {
				return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{}, RequestState: "opaque-✓"}, nil
			}
			if req.Params.RequestState != "opaque-✓" {
				t.Error("continuation state lost")
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil
		})
	session := connect(t, server, newClient(disabled), false)
	params := &mcp.CallToolParams{Name: "probe"}
	before := string(marshal(t, params))
	// Act.
	// SDK mutates params; an adapter must submit an owned copy.
	owned := *params
	result, err := session.CallTool(t.Context(), &owned)
	// Assert: no hidden retry in manual mode, no mutation in either mode.
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != wantCalls || result.NeedsInput() != disabled || string(marshal(t, params)) != before {
		t.Fatalf("calls=%d needsInput=%v params=%s", calls.Load(), result.NeedsInput(), marshal(t, params))
	}
	if disabled {
		if result.RequestState != "opaque-✓" || result.InputRequests == nil {
			t.Fatal("intermediate state lost")
		}
		// Act: only the host starts the next round.
		terminal, nextErr := session.CallTool(
			t.Context(),
			&mcp.CallToolParams{Name: "probe", RequestState: result.RequestState},
		)
		// Assert.
		if nextErr != nil || terminal.NeedsInput() || calls.Load() != 2 {
			t.Fatalf("manual continuation: %v calls=%d", nextErr, calls.Load())
		}
	}
}

func TestCharacterizationCallParamsMutation(t *testing.T) {
	// Arrange: caller-owned params; nil Arguments is allowed by the SDK.
	server := mcp.NewServer(&mcp.Implementation{Name: "params", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	session := connect(t, server, newClient(true), false)
	params := &mcp.CallToolParams{Name: "probe"}
	before := string(marshal(t, params))
	// Act.
	_, err := session.CallTool(t.Context(), params)
	// Assert: SDK modifies arguments and protocol metadata on caller-owned params.
	if err != nil {
		t.Fatal(err)
	}
	if string(marshal(t, params)) == before || params.Arguments == nil || params.Meta == nil {
		t.Fatal("reassess SDK params mutation")
	}
}

func TestCharacterizationMalformedHTTPResults(t *testing.T) {
	// Arrange: SDK serves real discovery/call HTTP traffic. A hostile endpoint
	// mutates only the tool response body, not a second protocol implementation.
	for _, fixture := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"missing_tag", func(raw []byte) []byte { return bytes.ReplaceAll(raw, []byte(`,"resultType":"complete"`), nil) }},
		{"unknown_tag", func(raw []byte) []byte {
			return bytes.ReplaceAll(raw, []byte(`"resultType":"complete"`), []byte(`"resultType":"nonsense"`))
		}},
		{"duplicate_key", func(raw []byte) []byte { return bytes.ReplaceAll(raw, []byte(`"a":1`), []byte(`"a":1,"a":2`)) }},
		{"unknown_field", func(raw []byte) []byte {
			return bytes.ReplaceAll(raw, []byte(`"structuredContent":`), []byte(`"vendorField":"keep","structuredContent":`))
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			result, mutations, err := malformedHTTPResult(t, fixture.mutate)
			// Assert: verify mutation actually hit the wire before characterizing acceptance.
			if mutations != 1 {
				t.Fatal("fixture did not mutate exactly one tool response")
			}
			if err != nil {
				t.Fatalf("reassess actual wire validation: %v", err)
			}
			out := object(t, marshal(t, result))
			if fixture.name == "duplicate_key" && string(out["structuredContent"]) != `{"a":2}` {
				t.Fatal("reassess duplicate collapse", string(out["structuredContent"]))
			}
			if _, present := out["vendorField"]; present {
				t.Fatal("reassess unknown-field retention")
			}
			t.Logf("accepted %s; output=%s", fixture.name, marshal(t, result))
		})
	}
}

func malformedHTTPResult(t *testing.T, mutate func([]byte) []byte) (*mcp.CallToolResult, int32, error) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "hostile", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: json.RawMessage(`{"a":1}`)}, nil
		})
	sdkHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	var mutated atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		sdkHandler.ServeHTTP(recorder, r)
		raw := recorder.Body.Bytes()
		if bytes.Contains(raw, []byte(`"structuredContent"`)) {
			replacement := mutate(raw)
			if !bytes.Equal(raw, replacement) {
				mutated.Add(1)
			}
			raw = replacement
		}
		maps.Copy(w.Header(), recorder.Header())
		w.Header().Del("Content-Length")
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(raw)
	}))
	t.Cleanup(host.Close)
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	t.Cleanup(cancel)
	session, err := newClient(true).Connect(ctx,
		&mcp.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	// Act.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "probe"})
	return result, mutated.Load(), err
}

func TestNegotiationFallsBack(t *testing.T) {
	// Arrange: default client's preference is newer than the only server version.
	server := mcp.NewServer(&mcp.Implementation{Name: "legacy", Version: "1"}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2025-11-25"},
	})
	// Act.
	session := connect(t, server, newClient(true), false)
	// Assert: capability checks cannot infer strict pinning from preference.
	if session.InitializeResult().ProtocolVersion != "2025-11-25" {
		t.Fatal(session.InitializeResult())
	}
}

func TestCharacterizationProgressAfterSDKTerminalBeforeMiddlewareReturns(t *testing.T) {
	// Arrange: public SDK middleware observes the terminal before outer CallTool returns.
	server := mcp.NewServer(&mcp.Implementation{Name: "ordering", Version: "1"}, nil)
	request := make(chan *mcp.CallToolRequest, 1)
	server.AddTool(
		&mcp.Tool{Name: "one", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			request <- req
			return &mcp.CallToolResult{}, nil
		},
	)
	progress := make(chan struct{}, 1)
	client := mcp.NewClient(
		&mcp.Implementation{Name: "host", Version: "1"},
		&mcp.ClientOptions{
			MultiRoundTrip:              &mcp.MultiRoundTripOptions{Disabled: true},
			ProgressNotificationHandler: func(context.Context, *mcp.ProgressNotificationClientRequest) { progress <- struct{}{} },
		},
	)
	terminal := make(chan struct{})
	release := make(chan struct{})
	client.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if method == "tools/call" {
				close(terminal)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return result, err
		}
	})
	session := connect(t, server, client, false)
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()
	complete := make(chan error, 1)
	// Act: deliver progress after SDK's terminal result but before middleware returns.
	go func() {
		params := &mcp.CallToolParams{Name: "one"}
		params.SetProgressToken("late-token")
		_, err := session.CallTool(ctx, params)
		complete <- err
	}()
	select {
	case <-terminal:
	case <-ctx.Done():
		t.Fatal("terminal not observed")
	}
	req := <-request
	if err := req.Session.NotifyProgress(
		ctx,
		&mcp.ProgressNotificationParams{ProgressToken: "late-token", Progress: 1},
	); err != nil {
		t.Fatal(err)
	}
	// Assert: SDK callbacks have no built-in request retirement at this boundary.
	select {
	case <-progress:
	case <-ctx.Done():
		t.Fatal("late progress was not delivered")
	}
	close(release)
	if err := <-complete; err != nil {
		t.Fatal(err)
	}
}
