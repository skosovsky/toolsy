package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type contractPending struct {
	id       json.RawMessage
	result   json.RawMessage
	err      error
	done     chan struct{}
	mu       sync.Mutex
	hooks    []func()
	complete bool
}

func newContractPending(id int64, result json.RawMessage, err error) *contractPending {
	done := make(chan struct{})
	close(done)
	return &contractPending{id: json.RawMessage(jsonNumber(id)), result: bytes.Clone(result), err: err, done: done}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (p *contractPending) ID() json.RawMessage { return bytes.Clone(p.id) }
func (p *contractPending) Await(context.Context) (json.RawMessage, error) {
	return bytes.Clone(p.result), p.err
}
func (p *contractPending) DeliveryDone() <-chan struct{} { return p.done }
func (*contractPending) WasSent() bool                   { return true }
func (*contractPending) CancelPending() bool             { return true }
func (p *contractPending) Deliver() error {
	p.completeCallbacks()
	return nil
}
func (*contractPending) Abort(error) error { return nil }

func (p *contractPending) OnComplete(hook func()) {
	p.mu.Lock()
	if !p.complete {
		p.hooks = append(p.hooks, hook)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	hook()
}

func (p *contractPending) completeCallbacks() {
	p.mu.Lock()
	if p.complete {
		p.mu.Unlock()
		return
	}
	p.complete = true
	hooks := append([]func(){}, p.hooks...)
	p.hooks = nil
	p.mu.Unlock()
	for _, hook := range hooks {
		hook()
	}
}

type contractRequest struct {
	method string
	params json.RawMessage
}

type contractTransport struct {
	mu       sync.Mutex
	nextID   int64
	requests []contractRequest
	notifies []contractRequest
	handlers map[string]NotificationHandler
	script   func(string, json.RawMessage) (json.RawMessage, error)
	closed   bool
}

func newContractTransport(script func(string, json.RawMessage) (json.RawMessage, error)) *contractTransport {
	return &contractTransport{handlers: make(map[string]NotificationHandler), script: script}
}

func (*contractTransport) Start(context.Context) error { return nil }
func (t *contractTransport) PrepareRequest(
	_ context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.nextID++
	id := t.nextID
	t.requests = append(t.requests, contractRequest{method: method, params: bytes.Clone(raw)})
	t.mu.Unlock()
	result, callErr := t.script(method, raw)
	return newContractPending(id, result, callErr), nil
}
func (t *contractTransport) Notify(_ context.Context, method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.notifies = append(t.notifies, contractRequest{method: method, params: raw})
	t.mu.Unlock()
	return nil
}
func (t *contractTransport) OnNotification(method string, handler NotificationHandler) {
	t.mu.Lock()
	t.handlers[method] = handler
	t.mu.Unlock()
}
func (t *contractTransport) Close() error {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	return nil
}

func (t *contractTransport) emit(method string, params any) {
	raw, _ := json.Marshal(params)
	t.mu.Lock()
	handler := t.handlers[method]
	t.mu.Unlock()
	if handler != nil {
		handler(raw)
	}
}

func completeDiscovery(capabilities ServerCapabilities) json.RawMessage {
	result := DiscoverResult{
		ResultType:        ResultTypeComplete,
		SupportedVersions: []string{ProtocolVersion},
		Capabilities:      capabilities,
		TTLMS:             JSONNumber("0"), CacheScope: CacheScopePrivate,
		Meta: ResultMeta{ServerInfo: &Implementation{
			Name: "contract-server", Version: "1.0.0",
		}},
	}
	raw, _ := json.Marshal(result)
	return raw
}

func TestConnectUsesStrictDiscoveryAndSelfDescribingMetadata(t *testing.T) {
	// Arrange.
	transport := newContractTransport(func(method string, params json.RawMessage) (json.RawMessage, error) {
		require.Equal(t, MethodServerDiscover, method)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(params, &fields))
		var meta map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(fields["_meta"], &meta))
		require.JSONEq(t, `"2026-07-28"`, string(meta[metaProtocolVersion]))
		require.JSONEq(t, `{}`, string(meta[metaClientCapabilities]))
		require.NotEmpty(t, meta[metaClientInfo])
		return completeDiscovery(ServerCapabilities{}), nil
	})

	// Act.
	client, err := Connect(context.Background(), transport)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, ProtocolVersion, client.ServerInfo().SupportedVersions[0])
	require.Len(t, transport.requests, 1)
	require.NoError(t, client.Close())
}

func TestConnectRejectsOldOnlyServerWithoutFallback(t *testing.T) {
	// Arrange.
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		require.Equal(t, MethodServerDiscover, method)
		return json.RawMessage(
			`{"resultType":"complete","supportedVersions":["2025-11-25"],"capabilities":{},"ttlMs":0,"cacheScope":"private"}`,
		), nil
	})

	// Act.
	client, err := Connect(context.Background(), transport)

	// Assert.
	require.Nil(t, client)
	var versionErr *ProtocolVersionError
	require.ErrorAs(t, err, &versionErr)
	require.Len(t, transport.requests, 1)
	require.Equal(t, MethodServerDiscover, transport.requests[0].method)
	require.True(t, transport.closed)
}

func TestRequestMetaRejectsReservedInjectionAndDuplicateKeys(t *testing.T) {
	// Arrange.
	meta := RequestMeta{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: ClientCapabilities{},
		ClientInfo:         &Implementation{Name: "client", Version: "1"},
		Extra:              Meta{metaProtocolVersion: json.RawMessage(`"forged"`)},
	}

	// Act.
	_, marshalErr := json.Marshal(meta)
	var decoded RequestMeta
	duplicateErr := json.Unmarshal([]byte(`{
		"io.modelcontextprotocol/protocolVersion":"2026-07-28",
		"io.modelcontextprotocol/protocolVersion":"2025-11-25",
		"io.modelcontextprotocol/clientCapabilities":{},
		"io.modelcontextprotocol/clientInfo":{"name":"c","version":"1"}
	}`), &decoded)

	// Assert.
	require.ErrorContains(t, marshalErr, "reserved key")
	require.ErrorContains(t, duplicateErr, "duplicate")
}

func TestProtocolRequiresCurrentResultAndCacheFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		into any
	}{
		{
			name: "discover resultType",
			raw:  `{"supportedVersions":["2026-07-28"],"capabilities":{},"ttlMs":0,"cacheScope":"private"}`,
			into: &DiscoverResult{},
		},
		{
			name: "tools ttl",
			raw:  `{"resultType":"complete","tools":[],"cacheScope":"private"}`,
			into: &ToolsListResult{},
		},
		{
			name: "resources cache scope",
			raw:  `{"resultType":"complete","resources":[],"ttlMs":0}`,
			into: &ResourcesListResult{},
		},
		{
			name: "templates ttl",
			raw:  `{"resultType":"complete","resourceTemplates":[],"cacheScope":"private"}`,
			into: &ResourceTemplatesListResult{},
		},
		{
			name: "prompts resultType",
			raw:  `{"prompts":[],"ttlMs":0,"cacheScope":"private"}`,
			into: &PromptsListResult{},
		},
		{
			name: "read negative ttl",
			raw:  `{"resultType":"complete","contents":[],"ttlMs":-1,"cacheScope":"private"}`,
			into: &ResourcesReadResult{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act.
			err := json.Unmarshal([]byte(test.raw), test.into)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestResultTypeAndStructuredContentAreStrictAndLossless(t *testing.T) {
	// Arrange.
	validValues := []string{`null`, `true`, `"value"`, `[1,9007199254740993]`, `{"n":9007199254740993}`}

	for _, structured := range validValues {
		// Act.
		var result CallToolResult
		err := json.Unmarshal(
			[]byte(`{"resultType":"complete","content":[],"structuredContent":`+structured+`}`),
			&result,
		)

		// Assert.
		require.NoError(t, err, structured)
		require.JSONEq(t, structured, string(result.StructuredContent))
	}

	var missing CallToolResult
	require.Error(t, json.Unmarshal([]byte(`{"content":[]}`), &missing))
	var unknown CallToolResult
	require.Error(t, json.Unmarshal([]byte(`{"resultType":"future","content":[]}`), &unknown))
	var input InputRequiredResult
	require.Error(t, json.Unmarshal([]byte(`{"resultType":"input_required"}`), &input))
	require.NoError(t, json.Unmarshal([]byte(`{"resultType":"input_required","requestState":"opaque"}`), &input))
}

func TestUndeclaredMRTRCapabilityDoesNotRetry(t *testing.T) {
	// Arrange.
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		require.Equal(t, MethodToolsCall, method)
		return json.RawMessage(
			`{"resultType":"input_required","inputRequests":{"confirm":{"method":"elicitation/create","params":{"message":"Continue?","requestedSchema":{"type":"object","properties":{"confirmed":{"type":"boolean"}}}}}}}`,
		), nil
	})
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	complete, input, err := client.CallTool(
		context.Background(),
		ToolsCallParams{Name: "delete", Arguments: json.RawMessage(`{}`)},
	)

	// Assert.
	var unsupported *UnsupportedFeatureError
	require.ErrorAs(t, err, &unsupported)
	require.Nil(t, complete)
	require.Nil(t, input)
	require.Len(t, transport.requests, 2)
	require.Equal(t, MethodToolsCall, transport.requests[1].method)
}

func TestTypedReservedRPCErrors(t *testing.T) {
	// Arrange.
	rpcErr := &RPCError{
		Code:    JSONRPCUnsupportedProtocolVersion,
		Message: "unsupported",
		Data:    json.RawMessage(`{"requested":"2026-07-28","supported":["future"]}`),
	}

	// Act.
	err := TypedRPCError(rpcErr)

	// Assert.
	var typed *UnsupportedProtocolVersionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, ProtocolVersion, typed.Requested)
}

func requestMetaForContract() *RequestMeta {
	return &RequestMeta{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: ClientCapabilities{},
		ClientInfo:         &Implementation{Name: "contract-client", Version: "1"},
	}
}

func TestStreamableHTTPUsesPostOnlyRoutingAndSentinelHeaders(t *testing.T) {
	// Arrange.
	var methods []string
	var captured http.Header
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		methods = append(methods, request.Method)
		captured = request.Header.Clone()
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(
			[]byte(
				`{"jsonrpc":"2.0","id":` + string(envelope.ID) + `,"result":{"resultType":"complete","content":[]}}`,
			),
		)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	require.NoError(t, transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"удалить": {
			{Header: "Tenant", Path: []string{"tenant"}, Type: "string"},
			{Header: "Count", Path: []string{"count"}, Type: "integer"},
		},
	}))

	// Act.
	pending, err := transport.PrepareRequest(context.Background(), MethodToolsCall, ToolsCallParams{
		Name: "удалить", Arguments: json.RawMessage(`{"tenant":" padded ","count":42}`), Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	_, err = pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.Equal(t, []string{http.MethodPost}, methods)
	require.Equal(t, ProtocolVersion, captured.Get("Mcp-Protocol-Version"))
	require.Equal(t, MethodToolsCall, captured.Get(mcpMethodHeader))
	require.Equal(t, "=?base64?0YPQtNCw0LvQuNGC0Yw=?=", captured.Get(mcpNameHeader))
	require.Equal(t, "=?base64?IHBhZGRlZCA=?=", captured.Get("Mcp-Param-Tenant"))
	require.Equal(t, "42", captured.Get("Mcp-Param-Count"))
	require.Empty(t, captured.Get("Mcp-Session-Id"))
	require.Empty(t, captured.Get("Last-Event-ID"))
	require.NoError(t, transport.Close())
}

func TestHTTPToolHeaderCompilerRejectsDynamicAndCaseCollidingAnnotations(t *testing.T) {
	// Arrange/Act.
	_, dynamicErr := compileHTTPToolHeaderBindings(json.RawMessage(`{
		"type":"object","allOf":[{"properties":{"x":{"type":"string","x-mcp-header":"X"}}}]
	}`))
	_, duplicateErr := compileHTTPToolHeaderBindings(json.RawMessage(`{
		"type":"object","properties":{
			"a":{"type":"string","x-mcp-header":"Tenant"},
			"b":{"type":"boolean","x-mcp-header":"tenant"}
		}
	}`))
	bindings, validErr := compileHTTPToolHeaderBindings(json.RawMessage(`{
		"type":"object","properties":{"nested":{"type":"object","properties":{
			"enabled":{"type":"boolean","x-mcp-header":"Enabled"}
		}}}
	}`))

	// Assert.
	require.Error(t, dynamicErr)
	require.ErrorContains(t, duplicateErr, "duplicate")
	require.Error(t, validErr)
	require.Empty(t, bindings)
}

func TestResourceAndPromptListAPIsPreserveCacheMetadata(t *testing.T) {
	// Arrange.
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(
				ServerCapabilities{Resources: &ResourcesCapability{}, Prompts: &PromptsCapability{}},
			), nil
		case MethodResourcesList:
			return json.RawMessage(`{"resultType":"complete","resources":[],"ttlMs":25,"cacheScope":"public"}`), nil
		case MethodResourceTemplatesList:
			return json.RawMessage(
				`{"resultType":"complete","resourceTemplates":[],"ttlMs":30,"cacheScope":"private"}`,
			), nil
		case MethodPromptsList:
			return json.RawMessage(`{"resultType":"complete","prompts":[],"ttlMs":35,"cacheScope":"private"}`), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	resources, resourcesErr := client.ListResources(context.Background(), "")
	templates, templatesErr := client.ListResourceTemplates(context.Background(), "")
	prompts, promptsErr := client.ListPrompts(context.Background(), "")

	// Assert.
	require.NoError(t, resourcesErr)
	require.NoError(t, templatesErr)
	require.NoError(t, promptsErr)
	require.Equal(t, JSONNumber("25"), resources.TTLMS)
	require.Equal(t, JSONNumber("30"), templates.TTLMS)
	require.Equal(t, JSONNumber("35"), prompts.TTLMS)
}

func TestSubscriptionRequiresAcknowledgementBeforeEvents(t *testing.T) {
	// Arrange.
	listenPending := &blockingContractPending{id: json.RawMessage(`2`), done: make(chan struct{})}
	transport := &subscriptionContractTransport{
		contractTransport: newContractTransport(func(_ string, _ json.RawMessage) (json.RawMessage, error) {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}), nil
		}),
		listen: listenPending,
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	subscription, err := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)

	// Act: event before ACK must terminate the subscription fail closed.
	transport.emit(MethodToolsListChanged, map[string]any{
		"_meta": map[string]any{metaSubscriptionID: json.Number("2")},
	})

	// Assert.
	require.Eventually(t, func() bool { return subscription.Err() != nil }, time.Second, 10*time.Millisecond)
	require.ErrorContains(t, subscription.Err(), "before acknowledgment")
	subscription.Close()
}

type blockingContractPending struct {
	id   json.RawMessage
	done chan struct{}
}

func (p *blockingContractPending) ID() json.RawMessage { return bytes.Clone(p.id) }
func (*blockingContractPending) Deliver() error        { return nil }
func (*blockingContractPending) Abort(error) error     { return nil }
func (p *blockingContractPending) Await(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-p.done:
		return nil, ErrTransportClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type subscriptionContractTransport struct {
	*contractTransport

	listen *blockingContractPending
}

func (t *subscriptionContractTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	if method == MethodSubscriptionsListen {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		t.requests = append(t.requests, contractRequest{method: method, params: raw})
		t.mu.Unlock()
		return t.listen, nil
	}
	return t.contractTransport.PrepareRequest(ctx, method, params)
}
