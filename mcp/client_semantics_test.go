package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type bindingContractTransport struct {
	*contractTransport

	bindingsMu sync.Mutex
	bindings   map[string][]HTTPToolHeaderBinding
}

type deliverHookPending struct {
	*contractPending

	hook func()
}

func (p *deliverHookPending) Deliver() error {
	p.hook()
	return nil
}

type deliverHookTransport struct {
	*contractTransport

	delivered atomic.Int64
}

func (t *deliverHookTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	pending, err := t.contractTransport.PrepareRequest(ctx, method, params)
	if err != nil || method != MethodSubscriptionsListen {
		return pending, err
	}
	contract := pending.(*contractPending)
	return &deliverHookPending{contractPending: contract, hook: func() {
		t.delivered.Add(1)
		t.emit(MethodSubscriptionsAcknowledged, SubscriptionsAcknowledgedParams{
			Notifications: SubscriptionFilter{ToolsListChanged: true},
			Meta:          Meta{metaSubscriptionID: contract.ID()},
		})
	}}, nil
}

type logHookPending struct {
	*contractPending

	onDeliver func()
	hooks     []func()
	hold      bool
}

func (p *logHookPending) OnComplete(hook func()) { p.hooks = append(p.hooks, hook) }
func (p *logHookPending) Deliver() error {
	p.onDeliver()
	if p.hold {
		return nil
	}
	for _, hook := range p.hooks {
		hook()
	}
	return nil
}

type contextualLogTransport struct {
	*contractTransport

	handler RequestScopedNotificationHandler
}

func (t *contextualLogTransport) OnRequestNotification(
	method string,
	handler RequestScopedNotificationHandler,
) {
	if method == MethodLogMessage {
		t.handler = handler
	}
}

func (t *contextualLogTransport) PrepareRequest(
	ctx context.Context,
	method string,
	params any,
) (PreparedRequest, error) {
	pending, err := t.contractTransport.PrepareRequest(ctx, method, params)
	if err != nil || method != MethodToolsList {
		return pending, err
	}
	contract := pending.(*contractPending)
	return &logHookPending{
		contractPending: contract,
		onDeliver: func() {
			raw, _ := json.Marshal(LogMessageParams{Level: "info", Data: json.RawMessage(`"http active"`)})
			t.handler(contract.ID(), raw)
		},
	}, nil
}

type cancellationPrepared struct {
	delivered chan struct{}
	sent      atomic.Bool
	claimed   atomic.Bool
	once      sync.Once
}

func (p *cancellationPrepared) ID() json.RawMessage { return json.RawMessage(`77`) }
func (p *cancellationPrepared) Await(ctx context.Context) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (p *cancellationPrepared) Deliver() error {
	p.sent.Store(true)
	p.once.Do(func() { close(p.delivered) })
	return nil
}
func (p *cancellationPrepared) Abort(error) error {
	p.once.Do(func() { close(p.delivered) })
	return nil
}
func (p *cancellationPrepared) DeliveryDone() <-chan struct{} { return p.delivered }
func (p *cancellationPrepared) WasSent() bool                 { return p.sent.Load() }
func (*cancellationPrepared) CancelPending() bool             { return true }
func (p *cancellationPrepared) ClaimCancellationNotification() bool {
	return p.claimed.CompareAndSwap(false, true)
}

type cancellationTransport struct {
	*contractTransport

	pending *cancellationPrepared
	count   atomic.Int64
}

func (t *cancellationTransport) PrepareRequest(
	context.Context,
	string,
	any,
) (PreparedRequest, error) {
	return t.pending, nil
}
func (t *cancellationTransport) Notify(ctx context.Context, method string, params any) error {
	if method == MethodCancelled {
		t.count.Add(1)
		return nil
	}
	return t.contractTransport.Notify(ctx, method, params)
}

func (t *bindingContractTransport) ReplaceToolHeaderBindings(
	bindings map[string][]HTTPToolHeaderBinding,
) error {
	t.bindingsMu.Lock()
	defer t.bindingsMu.Unlock()
	replacement := make(map[string][]HTTPToolHeaderBinding, len(bindings))
	for tool, toolBindings := range bindings {
		replacement[tool] = cloneHTTPToolHeaderBindings(toolBindings)
	}
	t.bindings = replacement
	return nil
}

func TestPrepareParamsRejectsUnsupportedShapeAndAcceptsLoggingOptIn(t *testing.T) {
	// Arrange.
	client := &Client{opts: ClientOptions{ClientInfo: Implementation{Name: "client", Version: "1"}}}

	// Act.
	_, shapeErr := client.prepareParams(map[string]any{"name": "tool"})
	loggingRaw, loggingErr := client.prepareParams(ToolsListParams{Meta: &RequestMeta{LogLevel: "info"}})

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, shapeErr, &invalid)
	require.NoError(t, loggingErr)
	require.Contains(t, string(loggingRaw), `"io.modelcontextprotocol/logLevel":"info"`)
	require.NotContains(t, string(loggingRaw), `"progressToken"`)
}

func TestSharedTransportRejectsRequestScopedLoggingBeforePrepare(t *testing.T) {
	// Arrange.
	base := newContractTransport(func(string, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	client := &Client{
		transport: base,
		opts:      ClientOptions{ClientInfo: Implementation{Name: "client", Version: "1"}},
		logger:    testLogger(),
	}

	// Act.
	_, err := client.requestAndAwait(
		context.Background(),
		MethodToolsList,
		ToolsListParams{Meta: &RequestMeta{LogLevel: "info"}},
	)

	// Assert.
	var unsupported *UnsupportedFeatureError
	require.ErrorAs(t, err, &unsupported)
	base.mu.Lock()
	require.Empty(t, base.requests)
	base.mu.Unlock()
}

func TestContextualHTTPLoggingDoesNotRequireProgressToken(t *testing.T) {
	// Arrange.
	var output bytes.Buffer
	base := newContractTransport(func(string, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	})
	transport := &contextualLogTransport{contractTransport: base}
	client := &Client{
		transport: transport,
		opts: ClientOptions{
			ClientInfo: Implementation{Name: "client", Version: "1"},
		},
		logger: slog.New(slog.NewTextHandler(&output, nil)),
	}
	client.registerHandlers()

	// Act.
	_, err := client.requestAndAwait(
		context.Background(),
		MethodToolsList,
		ToolsListParams{Meta: &RequestMeta{LogLevel: "info"}},
	)
	require.NoError(t, err)
	late, _ := json.Marshal(LogMessageParams{Level: "info", Data: json.RawMessage(`"late"`)})
	transport.handler(json.RawMessage(`1`), late)

	// Assert.
	require.Equal(t, 1, bytes.Count(output.Bytes(), []byte("mcp: request-scoped log")))
	base.mu.Lock()
	require.NotContains(t, string(base.requests[0].params), `"progressToken"`)
	base.mu.Unlock()
}

func TestAwaitCancellationNotifiesExactlyOnceAfterDelivery(t *testing.T) {
	// Arrange.
	pending := &cancellationPrepared{delivered: make(chan struct{})}
	transport := &cancellationTransport{
		contractTransport: newContractTransport(func(string, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		}),
		pending: pending,
	}
	client := &Client{
		transport: transport,
		opts:      ClientOptions{ClientInfo: Implementation{Name: "client", Version: "1"}},
		logger:    testLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.requestAndAwait(ctx, MethodToolsList, ToolsListParams{})
		done <- err
	}()
	<-pending.delivered

	// Act.
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	client.notifyCancelledPending(ctx, pending, "duplicate racing cancellation")

	// Assert.
	require.EqualValues(t, 1, transport.count.Load())
}

func TestDuplicateActiveRequestLogIDIsRejected(t *testing.T) {
	// Arrange.
	transport := &contextualLogTransport{contractTransport: newContractTransport(nil)}
	client := &Client{transport: transport}
	first := &logHookPending{
		contractPending: newContractPending(1, nil, nil),
		onDeliver:       func() {},
		hold:            true,
	}
	second := &logHookPending{
		contractPending: newContractPending(1, nil, nil),
		onDeliver:       func() {},
	}
	scope := requestLogScope{enabled: true}
	require.NoError(t, client.deliverPrepared(first, scope))

	// Act.
	err := client.deliverPrepared(second, scope)

	// Assert.
	require.ErrorContains(t, err, "correlation token is already active")
	for _, hook := range first.hooks {
		hook()
	}
	_, retained := client.requestLogs.Load("r:n:1")
	require.False(t, retained)
}

func TestProgressTokenKeyPreservesJSONDiscriminator(t *testing.T) {
	// Arrange.
	stringToken := NewStringProgressToken("1")
	numericToken := NewIntegerProgressToken(1)

	// Act.
	stringKey, stringErr := progressTokenKey(stringToken)
	numericKey, numericErr := progressTokenKey(numericToken)

	// Assert.
	require.NoError(t, stringErr)
	require.NoError(t, numericErr)
	require.Equal(t, "s:1", stringKey)
	require.Equal(t, "n:1", numericKey)
	require.NotEqual(t, stringKey, numericKey)
}

func TestListToolsFiltersAndRegistersHeaderBindings(t *testing.T) {
	// Arrange.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		case MethodToolsList:
			return json.RawMessage(`{
				"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[
					{"name":"valid","inputSchema":{"type":"object","properties":{"tenant":{"type":"string","x-mcp-header":"Tenant"}}}},
					{"name":"invalid","inputSchema":{"type":"object","properties":{"tenant":{"type":"number","x-mcp-header":"Tenant"}}}}
				]
			}`), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	page, err := client.ListToolsPage(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, page.Tools, 1)
	require.Empty(t, transport.bindings)
	result, err := client.DiscoverTools(context.Background())

	// Assert.
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	require.Equal(t, "valid", result.Tools[0].Name)
	require.Equal(
		t,
		[]HTTPToolHeaderBinding{{Header: "Tenant", Path: []string{"tenant"}, Type: "string"}},
		transport.bindings["valid"],
	)
	require.Empty(t, transport.bindings["invalid"])

	// Act: an invalidation makes every previously compiled routing binding stale.
	client.advanceInvalidation(InvalidationTools, "", "")

	// Assert: no transport-level binding survives the generation boundary.
	transport.bindingsMu.Lock()
	require.Empty(t, transport.bindings["valid"])
	transport.bindingsMu.Unlock()
}

func TestHTTPDirectCallRequiresCurrentAuthoritativeToolDescriptor(t *testing.T) {
	// Arrange.
	var calls atomic.Int64
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		case MethodToolsList:
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"plain","inputSchema":{"type":"object"}}]}`,
			), nil
		case MethodToolsCall:
			calls.Add(1)
			return json.RawMessage(`{"resultType":"complete","content":[]}`), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act and assert: no descriptor means fail closed before RPC.
	_, _, beforeErr := client.CallTool(context.Background(), ToolsCallParams{Name: "plain", Arguments: []byte(`{}`)})
	require.Error(t, beforeErr)
	require.Zero(t, calls.Load())

	// Act and assert: a successfully listed known-empty binding is authoritative.
	_, err = client.DiscoverTools(context.Background())
	require.NoError(t, err)
	transport.bindingsMu.Lock()
	_, knownEmpty := transport.bindings["plain"]
	transport.bindingsMu.Unlock()
	require.True(t, knownEmpty)
	_, _, callErr := client.CallTool(context.Background(), ToolsCallParams{Name: "plain", Arguments: []byte(`{}`)})
	require.NoError(t, callErr)
	require.EqualValues(t, 1, calls.Load())

	// Act and assert: invalidation revokes authority before another RPC can start.
	client.advanceInvalidation(InvalidationTools, "", "")
	_, _, staleErr := client.CallTool(context.Background(), ToolsCallParams{Name: "plain", Arguments: []byte(`{}`)})
	require.Error(t, staleErr)
	require.EqualValues(t, 1, calls.Load())
}

func TestDuplicateToolNamesNeverBecomeRoutingAuthority(t *testing.T) {
	// Arrange.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		case MethodToolsList:
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"dup","inputSchema":{"type":"object"}},{"name":"dup","inputSchema":{"type":"object","properties":{"tenant":{"type":"string","x-mcp-header":"Tenant"}}}}]}`,
			), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	_, err = client.DiscoverTools(context.Background())

	// Assert.
	require.ErrorContains(t, err, "duplicate tool name")
	transport.bindingsMu.Lock()
	require.Empty(t, transport.bindings)
	transport.bindingsMu.Unlock()
}

func TestPaginatedToolAuthorityPublishesOnlyAfterCompleteTransaction(t *testing.T) {
	// Arrange.
	var phase atomic.Int64
	base := newContractTransport(func(method string, raw json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		}
		if method != MethodToolsList {
			return nil, errors.New("unexpected method")
		}
		var params ToolsListParams
		require.NoError(t, json.Unmarshal(raw, &params))
		switch phase.Load() {
		case 0:
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"old","inputSchema":{"type":"object"}}]}`,
			), nil
		case 1:
			if params.Cursor == "" {
				return json.RawMessage(
					`{"resultType":"complete","ttlMs":0,"cacheScope":"private","nextCursor":"p2","tools":[{"name":"new","inputSchema":{"type":"object"}}]}`,
				), nil
			}
			return nil, errors.New("late page failed")
		case 2:
			if params.Cursor == "" {
				return json.RawMessage(
					`{"resultType":"complete","ttlMs":0,"cacheScope":"private","nextCursor":"p2","tools":[{"name":"new","inputSchema":{"type":"object"}}]}`,
				), nil
			}
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"new","inputSchema":{"type":"object"}}]}`,
			), nil
		default:
			if params.Cursor == "" {
				return json.RawMessage(
					`{"resultType":"complete","ttlMs":0,"cacheScope":"private","nextCursor":"p2","tools":[{"name":"new","inputSchema":{"type":"object"}}]}`,
				), nil
			}
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"second","inputSchema":{"type":"object"}}]}`,
			), nil
		}
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	_, err = client.DiscoverTools(context.Background())
	require.NoError(t, err)

	consume := func() error {
		for _, iterErr := range client.Discover(context.Background()) {
			if iterErr != nil {
				return iterErr
			}
		}
		return nil
	}

	// Act and assert: a late-page failure cannot publish the first page.
	phase.Store(1)
	require.ErrorContains(t, consume(), "late page failed")
	transport.bindingsMu.Lock()
	_, oldAfterFailure := transport.bindings["old"]
	_, newAfterFailure := transport.bindings["new"]
	transport.bindingsMu.Unlock()
	require.True(t, oldAfterFailure)
	require.False(t, newAfterFailure)

	// Act and assert: a cross-page duplicate is equally transactional.
	phase.Store(2)
	require.ErrorContains(t, consume(), "duplicate tool name")
	transport.bindingsMu.Lock()
	_, oldAfterDuplicate := transport.bindings["old"]
	transport.bindingsMu.Unlock()
	require.True(t, oldAfterDuplicate)

	// Act and assert: only the complete successful snapshot replaces old authority.
	phase.Store(3)
	require.NoError(t, consume())
	transport.bindingsMu.Lock()
	_, staleOld := transport.bindings["old"]
	_, currentNew := transport.bindings["new"]
	_, currentSecond := transport.bindings["second"]
	transport.bindingsMu.Unlock()
	require.False(t, staleOld)
	require.True(t, currentNew)
	require.True(t, currentSecond)
}

func TestPublicCallToolValidatesAuthoritativeOutputSchema(t *testing.T) {
	// Arrange.
	invalidResult := json.RawMessage(
		`{"resultType":"complete","isError":true,"content":[],"structuredContent":{"value":"not-an-integer"}}`,
	)
	outputSchema := json.RawMessage(
		`{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}`,
	)
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		case MethodToolsList:
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[{"name":"typed","inputSchema":{"type":"object"},"outputSchema":{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}}]}`,
			), nil
		case MethodToolsCall:
			return invalidResult, nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	transport := &bindingContractTransport{contractTransport: base, bindings: make(map[string][]HTTPToolHeaderBinding)}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	_, err = client.DiscoverTools(context.Background())
	require.NoError(t, err)

	// Act.
	_, _, err = client.CallTool(context.Background(), ToolsCallParams{Name: "typed", Arguments: []byte(`{}`)})

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, structuredContentSubject)
	validator, compileErr := compileOutputSchema(outputSchema)
	require.NoError(t, compileErr)
	_, proxyErr := buildToolResultChunk("typed", invalidResult, validator)
	require.ErrorAs(t, proxyErr, &invalid)
}

func TestWithClientInfoDeepClonesExtensionMetadata(t *testing.T) {
	// Arrange.
	info := Implementation{
		Name:    "client",
		Version: "1",
		Extra:   Meta{"vendor": json.RawMessage(`{"value":1}`)},
		Icons: []Icon{{
			Src:   "https://example.test/icon.png",
			Extra: Meta{"vendor": json.RawMessage(`{"icon":1}`)},
		}},
	}
	var options ClientOptions
	WithClientInfo(info)(&options)

	// Act.
	info.Extra["vendor"][9] = '9'
	info.Icons[0].Extra["vendor"][8] = '9'

	// Assert.
	require.JSONEq(t, `{"value":1}`, string(options.ClientInfo.Extra["vendor"]))
	require.JSONEq(t, `{"icon":1}`, string(options.ClientInfo.Icons[0].Extra["vendor"]))
}

func TestConnectResnapshotsClientInfoAfterCustomOption(t *testing.T) {
	// Arrange.
	info := Implementation{
		Name:    "custom",
		Version: "1",
		Extra:   Meta{"vendor": json.RawMessage(`{"value":1}`)},
		Icons: []Icon{{
			Src:   "https://example.test/icon.png",
			Extra: Meta{"vendor": json.RawMessage(`{"icon":1}`)},
		}},
	}
	customOption := func(options *ClientOptions) { options.ClientInfo = info }
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{}), nil
		}
		return nil, errors.New("unexpected method")
	})
	client, err := Connect(context.Background(), transport, customOption)
	require.NoError(t, err)

	// Act.
	info.Extra["vendor"][9] = '9'
	info.Icons[0].Extra["vendor"][8] = '9'

	// Assert.
	require.JSONEq(t, `{"value":1}`, string(client.opts.ClientInfo.Extra["vendor"]))
	require.JSONEq(t, `{"icon":1}`, string(client.opts.ClientInfo.Icons[0].Extra["vendor"]))
}

func TestListenSnapshotsFilterSlicesAndExtensionMetadata(t *testing.T) {
	// Arrange.
	listenPending := &blockingContractPending{id: json.RawMessage(`2`), done: make(chan struct{})}
	transport := &subscriptionContractTransport{
		contractTransport: newContractTransport(func(_ string, _ json.RawMessage) (json.RawMessage, error) {
			return completeDiscovery(ServerCapabilities{
				Resources: &ResourcesCapability{Subscribe: true},
			}), nil
		}),
		listen: listenPending,
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	filter := SubscriptionFilter{
		ResourceSubscriptions: []string{"file:///original"},
		Extra:                 Meta{"vendor": json.RawMessage(`{"value":1}`)},
	}
	_, err = client.Listen(context.Background(), filter)
	require.NoError(t, err)

	// Act.
	filter.ResourceSubscriptions[0] = "file:///mutated"
	filter.Extra["vendor"][9] = '9'

	// Assert.
	client.mu.RLock()
	state := client.subscriptions["n:2"]
	client.mu.RUnlock()
	require.NotNil(t, state)
	state.mu.RLock()
	require.Equal(t, []string{"file:///original"}, state.requested.ResourceSubscriptions)
	require.JSONEq(t, `{"value":1}`, string(state.requested.Extra["vendor"]))
	state.mu.RUnlock()
	state.cancel()
}

func TestSubscriptionResourceViolationDoesNotDeadlock(t *testing.T) {
	// Arrange.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &subscriptionState{
		key:      "s:subscription",
		publicID: "subscription",
		acked:    true,
		effective: SubscriptionFilter{
			ResourceSubscriptions: []string{"file:///allowed"},
		},
		events: make(chan Invalidation, 1),
		done:   make(chan struct{}),
		cancel: cancel,
	}
	client := &Client{
		logger:        testLogger(),
		subscriptions: map[string]*subscriptionState{state.key: state},
		invalidations: make(chan Invalidation, 1),
	}
	done := make(chan struct{})

	// Act.
	go func() {
		client.handleSubscriptionInvalidation(
			InvalidationResource,
			json.RawMessage(
				`{"uri":"file:///denied","_meta":{"io.modelcontextprotocol/subscriptionId":"subscription"}}`,
			),
		)
		close(done)
	}()

	// Assert.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resource invalidation deadlocked")
	}
	state.mu.RLock()
	err := state.err
	state.mu.RUnlock()
	require.ErrorContains(t, err, "outside effective")
}

func TestEffectiveSubscriptionFilterMustBeStrictSubset(t *testing.T) {
	// Arrange.
	requested := SubscriptionFilter{ToolsListChanged: true, ResourceSubscriptions: []string{"file:///one"}}

	// Act.
	expandedErr := validateEffectiveSubscriptionFilter(
		requested,
		SubscriptionFilter{PromptsListChanged: true},
	)
	resourceErr := validateEffectiveSubscriptionFilter(
		requested,
		SubscriptionFilter{ResourceSubscriptions: []string{"file:///two"}},
	)
	validErr := validateEffectiveSubscriptionFilter(
		requested,
		SubscriptionFilter{ToolsListChanged: true},
	)

	// Assert.
	require.Error(t, expandedErr)
	require.Error(t, resourceErr)
	require.NoError(t, validErr)
}

func TestListenRegistersStateBeforeRequestDelivery(t *testing.T) {
	// Arrange: Deliver synchronously emits the acknowledgment, modeling a peer
	// that answers before the transport method returns to the client.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: true},
			}), nil
		case MethodSubscriptionsListen:
			return json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","_meta":{"io.modelcontextprotocol/subscriptionId":2}}`,
			), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	client, err := Connect(context.Background(), &deliverHookTransport{contractTransport: base})
	require.NoError(t, err)

	// Act.
	subscription, err := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("subscription did not reach its terminal result")
	}

	// Assert.
	require.NoError(t, subscription.Err())
}

func TestListenClosedClientAbortsWithoutWireDelivery(t *testing.T) {
	// Arrange.
	base := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: true},
			}), nil
		}
		return json.RawMessage(`{}`), nil
	})
	transport := &deliverHookTransport{contractTransport: base}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	client.mu.Lock()
	client.closed = true
	client.mu.Unlock()

	// Act.
	_, err = client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})

	// Assert.
	require.ErrorIs(t, err, ErrTransportClosed)
	require.Zero(t, transport.delivered.Load())
}

func TestDirectAwaitMapsReservedRPCError(t *testing.T) {
	// Arrange.
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		if method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Resources: &ResourcesCapability{}}), nil
		}
		return nil, &RPCError{Code: JSONRPCHeaderMismatch, Message: "wrong routing header"}
	})
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	_, err = client.ListResources(context.Background(), "")

	// Assert.
	var typed *HeaderMismatchError
	require.ErrorAs(t, err, &typed)
}

func TestResourceProxyRejectsStaleGenerationBeforeRPC(t *testing.T) {
	// Arrange.
	client := &Client{
		ready: true,
		server: DiscoverResult{Capabilities: ServerCapabilities{
			Resources: &ResourcesCapability{},
		}},
	}
	proxy, err := client.GetResourceTool()
	require.NoError(t, err)
	client.resourceGeneration.Add(1)

	// Act.
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"uri":"file:///resource"}`)},
		func(toolsy.Chunk) error { return nil },
	)

	// Assert.
	var stale *StaleDiscoveryError
	require.ErrorAs(t, err, &stale)
}

func TestDirectToolCallRejectsInvalidationDuringRPC(t *testing.T) {
	// Arrange.
	var client *Client
	transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
		switch method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil
		case MethodToolsCall:
			client.advanceInvalidation(InvalidationTools, "", "")
			return json.RawMessage(
				`{"resultType":"complete","content":[{"type":"text","text":"obsolete"}]}`,
			), nil
		default:
			return nil, errors.New("unexpected method")
		}
	})
	var err error
	client, err = Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	_, _, err = client.CallTool(context.Background(), ToolsCallParams{Name: "lookup", Arguments: []byte(`{}`)})

	// Assert.
	var stale *StaleDiscoveryError
	require.ErrorAs(t, err, &stale)
}

func TestDirectListAPIsRejectInvalidationDuringRPC(t *testing.T) {
	tests := []struct {
		name         string
		capabilities ServerCapabilities
		method       string
		result       json.RawMessage
		invalidate   InvalidationKind
		call         func(*Client) error
	}{
		{
			name:         "resources",
			capabilities: ServerCapabilities{Resources: &ResourcesCapability{}},
			method:       MethodResourcesList,
			result:       json.RawMessage(`{"resultType":"complete","ttlMs":0,"cacheScope":"private","resources":[]}`),
			invalidate:   InvalidationResources,
			call: func(client *Client) error {
				_, err := client.ListResources(context.Background(), "")
				return err
			},
		},
		{
			name:         "resource templates",
			capabilities: ServerCapabilities{Resources: &ResourcesCapability{}},
			method:       MethodResourceTemplatesList,
			result: json.RawMessage(
				`{"resultType":"complete","ttlMs":0,"cacheScope":"private","resourceTemplates":[]}`,
			),
			invalidate: InvalidationResources,
			call: func(client *Client) error {
				_, err := client.ListResourceTemplates(context.Background(), "")
				return err
			},
		},
		{
			name:         "prompts",
			capabilities: ServerCapabilities{Prompts: &PromptsCapability{}},
			method:       MethodPromptsList,
			result:       json.RawMessage(`{"resultType":"complete","ttlMs":0,"cacheScope":"private","prompts":[]}`),
			invalidate:   InvalidationPrompts,
			call: func(client *Client) error {
				_, err := client.ListPrompts(context.Background(), "")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			var client *Client
			transport := newContractTransport(func(method string, _ json.RawMessage) (json.RawMessage, error) {
				if method == MethodServerDiscover {
					return completeDiscovery(test.capabilities), nil
				}
				if method != test.method {
					return nil, errors.New("unexpected method")
				}
				client.advanceInvalidation(test.invalidate, "", "")
				return test.result, nil
			})
			var err error
			client, err = Connect(context.Background(), transport)
			require.NoError(t, err)

			// Act.
			err = test.call(client)

			// Assert.
			var stale *StaleDiscoveryError
			require.ErrorAs(t, err, &stale)
		})
	}
}

func TestResourceSubscriptionAllowsOnlyExactOrPathDescendantURI(t *testing.T) {
	// Arrange.
	subscriptions := []string{"file:///workspace/root", "https://example.test/api/"}

	// Act and assert.
	require.True(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root"))
	require.True(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root/child.json"))
	require.True(t, resourceSubscriptionAllows(subscriptions, "https://example.test/api/v1/item"))
	require.False(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root-escape"))
	require.False(t, resourceSubscriptionAllows(subscriptions, "https://evil.test/api/v1/item"))
	require.False(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root/child.json?version=2"))
	require.False(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root/../escape"))
	require.False(t, resourceSubscriptionAllows(subscriptions, "file:///workspace/root/%2e%2e/escape"))
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
