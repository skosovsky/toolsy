package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestClient_InvalidListChangedMetaDoesNotAdvanceGeneration(t *testing.T) {
	// Arrange.
	client := &Client{
		logger:        slog.Default(),
		invalidations: make(chan Invalidation, 1),
		server: DiscoverResult{Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: true},
		}},
		ready: true,
	}
	key, err := rpcIDKey(json.RawMessage(`2`))
	require.NoError(t, err)
	stateCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	state := &subscriptionState{
		key: key, publicID: "2", acked: true,
		requested: SubscriptionFilter{ToolsListChanged: true},
		effective: SubscriptionFilter{ToolsListChanged: true},
		events:    make(chan Invalidation, 1), cancel: cancel,
	}
	client.subscriptions = map[string]*subscriptionState{key: state}
	// A routable positive control proves this is not an unknown-ID no-op.
	control := &Client{logger: slog.Default(), invalidations: make(chan Invalidation, 1),
		subscriptions: map[string]*subscriptionState{key: state}}
	control.handleSubscriptionInvalidation(
		InvalidationTools,
		json.RawMessage(`{"_meta":{"io.modelcontextprotocol/subscriptionId":2}}`),
	)
	require.EqualValues(t, 1, control.toolGeneration.Load())
	require.Len(t, control.invalidations, 1)
	require.Len(t, state.events, 1)
	<-state.events

	// Act.
	client.handleSubscriptionInvalidation(InvalidationTools, json.RawMessage(`{"_meta":null}`))

	// Assert.
	require.Zero(t, client.toolGeneration.Load())
	require.Empty(t, client.invalidations)
	require.Empty(t, state.events)
	require.ErrorContains(t, state.err, "missing metadata")
	require.ErrorIs(t, stateCtx.Err(), context.Canceled)
}

func TestClient_SubscribeResourceRequiresObjectResultBeforeMutation(t *testing.T) {
	for _, fixture := range []string{"null", `[]`, `42`, `"scalar"`} {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			transport := newFakeTransport()
			transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
				if request.Method == MethodServerDiscover {
					return completeDiscovery(ServerCapabilities{
						Resources: &ResourcesCapability{Subscribe: true},
					}), nil, true
				}
				return json.RawMessage(fixture), nil, true
			}
			client, err := Connect(context.Background(), transport)
			require.NoError(t, err)

			// Act.
			subscription, listenErr := client.Listen(
				context.Background(),
				SubscriptionFilter{ResourceSubscriptions: []string{"file:///resource"}},
			)
			require.NoError(t, listenErr)
			select {
			case <-subscription.Done():
			case <-time.After(time.Second):
				t.Fatal("invalid subscription terminal did not settle")
			}
			err = subscription.Err()

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			client.mu.RLock()
			subscribed := len(client.subscriptions) != 0
			client.mu.RUnlock()
			require.False(t, subscribed)
			require.NoError(t, client.Close())
		})
	}
}

func TestClient_SubscribeResourceAcceptsExtendedEmptyResult(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{
				Resources: &ResourcesCapability{Subscribe: true},
			}), nil, true
		}
		require.NoError(t, transport.emit(MethodSubscriptionsAcknowledged, map[string]any{
			"notifications": SubscriptionFilter{ResourceSubscriptions: []string{"file:///resource"}},
			"_meta":         Meta{metaSubscriptionID: request.ID},
		}))
		return json.RawMessage(
			`{"resultType":"complete","_meta":{"io.modelcontextprotocol/subscriptionId":` + string(
				request.ID,
			) + `,"trace":"x"},"extension":{"ok":true}}`,
		), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	subscription, err := client.Listen(
		context.Background(),
		SubscriptionFilter{ResourceSubscriptions: []string{"file:///resource"}},
	)

	// Assert.
	require.NoError(t, err)
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("subscription terminal did not settle")
	}
	require.NoError(t, subscription.Err())
	require.NoError(t, client.Close())
}

func TestClient_CloseIsConcurrentIdempotentAndClosesEventChannels(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Resources: &ResourcesCapability{Subscribe: true}}), nil, true
		}
		require.NoError(
			t,
			transport.emit(
				MethodSubscriptionsAcknowledged,
				map[string]any{
					"notifications": SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
					"_meta":         Meta{metaSubscriptionID: request.ID},
				},
			),
		)
		return nil, nil, false
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	subscription, err := client.Listen(
		context.Background(),
		SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
	)
	require.NoError(t, err)
	const callers = 32
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)

	// Act.
	for range callers {
		go func() {
			defer wait.Done()
			errs <- client.Close()
		}()
	}
	wait.Wait()
	close(errs)

	// Assert.
	for closeErr := range errs {
		require.NoError(t, closeErr)
	}
	require.EqualValues(t, 1, transport.closeCalls.Load())
	_, invalidationsOpen := <-client.Invalidations()
	require.False(t, invalidationsOpen)
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("client close did not terminate its subscription")
	}
	_, subscriptionOpen := <-subscription.Events
	require.False(t, subscriptionOpen)
	// Logs use the host logger; no obsolete global log channel remains.
}

func TestClient_ToolDescriptorFailuresAreFailClosed(t *testing.T) {
	// Arrange.
	client := &Client{}
	cases := []struct {
		name string
		tool MCPTool
		as   any
	}{
		{name: "missing input schema", tool: MCPTool{Name: "valid"}, as: new(*InvalidPayloadError)},
		{
			name: "null input schema",
			tool: MCPTool{Name: "valid", InputSchema: json.RawMessage(`null`)},
			as:   new(*InvalidPayloadError),
		},
		{
			name: "invalid name",
			tool: MCPTool{Name: "has space", InputSchema: json.RawMessage(`{"type":"object"}`)},
			as:   new(*InvalidPayloadError),
		},
		{
			name: "required tasks",
			tool: MCPTool{
				Name:        "valid",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				Extra:       Meta{"execution": json.RawMessage(`{"taskSupport":"required"}`)},
			},
			as: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			proxy, err := client.toolToProxyAtGeneration(context.Background(), tc.tool, 0)

			// Assert.
			if tc.as == nil {
				require.NoError(t, err)
				require.NotNil(t, proxy)
				raw, marshalErr := json.Marshal(tc.tool)
				require.NoError(t, marshalErr)
				require.JSONEq(t, `{"taskSupport":"required"}`, string(mustObjectField(t, raw, "execution")))
				return
			}
			require.Error(t, err)
			switch tc.as.(type) {
			case **InvalidPayloadError:
				var target *InvalidPayloadError
				require.ErrorAs(t, err, &target)
			case **UnsupportedFeatureError:
				var target *UnsupportedFeatureError
				require.ErrorAs(t, err, &target)
			}
		})
	}
}

func TestClient_ToolsListChangedMakesExistingProxyStale(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(
				ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}},
			), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					ResultType: ResultTypeComplete,
					TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
					Tools: []MCPTool{
						{Name: "stale", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		case MethodSubscriptionsListen:
			require.NoError(
				t,
				transport.emit(
					MethodSubscriptionsAcknowledged,
					map[string]any{
						"notifications": SubscriptionFilter{ToolsListChanged: true},
						"_meta":         Meta{metaSubscriptionID: request.ID},
					},
				),
			)
			return nil, nil, false
		default:
			return nil, errors.New("stale proxy must not call transport"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var proxy toolsy.Tool
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	for item, iterErr := range client.Discover(context.Background()) {
		require.NoError(t, iterErr)
		proxy = item
	}
	subscription, err := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)

	// Act.
	require.NoError(
		t,
		transport.emit(
			MethodToolsListChanged,
			map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(subscription.ID)}},
		),
	)
	err = proxy.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)

	// Assert.
	var staleErr *StaleDiscoveryError
	require.ErrorAs(t, err, &staleErr)
	require.Equal(t, uint64(1), client.DiscoveryGeneration(InvalidationTools))
	select {
	case event := <-client.Invalidations():
		require.Equal(t, InvalidationTools, event.Kind)
	case <-time.After(time.Second):
		t.Fatal("missing invalidation event")
	}
}

func TestClient_ContradictingInvalidationIsIgnored(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return completeDiscovery(
			ServerCapabilities{Tools: &ToolsCapability{ListChanged: false}},
		), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	subscription, listenErr := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.Nil(t, subscription)
	var capabilityErr *CapabilityError
	require.ErrorAs(t, listenErr, &capabilityErr)
	require.NoError(
		t,
		transport.emit(MethodToolsListChanged, map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(`2`)}}),
	)

	// Assert.
	require.Zero(t, client.DiscoveryGeneration(InvalidationTools))
	require.Empty(t, client.invalidations)
	require.Len(t, transport.requests, 1, "unsupported subscription must not reach the wire")
	require.NoError(t, client.Close())
}

func TestClient_ResourceUpdateRequiresNegotiatedSubscription(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(
				ServerCapabilities{Resources: &ResourcesCapability{Subscribe: true}},
			), nil, true
		case MethodSubscriptionsListen:
			require.NoError(
				t,
				transport.emit(
					MethodSubscriptionsAcknowledged,
					map[string]any{
						"notifications": SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
						"_meta":         Meta{metaSubscriptionID: request.ID},
					},
				),
			)
			return nil, nil, false
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	subscription, err := client.Listen(
		context.Background(),
		SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	// Act.
	require.NoError(
		t,
		transport.emit(
			MethodResourceUpdated,
			ResourceUpdatedParams{
				URI:  "file:///watched",
				Meta: Meta{metaSubscriptionID: json.RawMessage(subscription.ID)},
			},
		),
	)

	// Assert.
	select {
	case event := <-client.Invalidations():
		require.Equal(t, InvalidationResource, event.Kind)
		require.Equal(t, "file:///watched", event.URI)
	case <-time.After(time.Second):
		t.Fatal("missing resource update")
	}
}

func TestClient_ResourceSubscriptionAcceptsSubResourceUpdate(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(
				ServerCapabilities{Resources: &ResourcesCapability{Subscribe: true}},
			), nil, true
		case MethodSubscriptionsListen:
			require.NoError(
				t,
				transport.emit(
					MethodSubscriptionsAcknowledged,
					map[string]any{
						"notifications": SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
						"_meta":         Meta{metaSubscriptionID: request.ID},
					},
				),
			)
			return nil, nil, false
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	subscription, err := client.Listen(
		context.Background(),
		SubscriptionFilter{ResourceSubscriptions: []string{"file:///watched"}},
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	// Act.
	require.NoError(
		t,
		transport.emit(
			MethodResourceUpdated,
			ResourceUpdatedParams{
				URI:  "file:///watched/child",
				Meta: Meta{metaSubscriptionID: json.RawMessage(subscription.ID)},
			},
		),
	)

	// Assert.
	select {
	case event := <-client.Invalidations():
		require.Equal(t, InvalidationResource, event.Kind)
		require.Equal(t, "file:///watched/child", event.URI)
	case <-time.After(time.Second):
		t.Fatal("missing sub-resource update")
	}
}

func TestResourceUpdateMatchesSubscriptionURIBoundaries(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name         string
		subscription string
		updated      string
		want         bool
	}{
		{name: "exact", subscription: "file:///watched", updated: "file:///watched", want: true},
		{name: "child", subscription: "file:///watched", updated: "file:///watched/child", want: true},
		{name: "sibling", subscription: "file:///watched", updated: "file:///unrelated"},
		{name: "prefix collision", subscription: "file:///watched", updated: "file:///watchedness"},
		{
			name:         "host case normalized",
			subscription: "https://EXAMPLE.com/root?v=1",
			updated:      "https://example.com/root?v=1",
			want:         true,
		},
		{
			name:         "dot segment escapes subtree",
			subscription: "file:///watched",
			updated:      "file:///watched/../other",
		},
		{
			name:         "encoded dot segments escape subtree",
			subscription: "file:///watched",
			updated:      "file:///watched/%2e%2E/other",
		},
		{
			name:         "encoded unreserved path",
			subscription: "file:///watched",
			updated:      "file:///%77atched/child",
			want:         true,
		},
		{
			name:         "encoded slash remains segment data",
			subscription: "file:///watched",
			updated:      "file:///watched%2fchild",
		},
		{
			name:         "default HTTPS port",
			subscription: "https://example.com:443/root",
			updated:      "https://example.com/root/child",
			want:         true,
		},
		{
			name:         "repeated slash remains distinct",
			subscription: "https://example.com/root/private",
			updated:      "https://example.com/root//private",
		},
		{
			name:         "reserved fragment escape remains distinct",
			subscription: "https://example.com/root#%2F",
			updated:      "https://example.com/root#/",
		},
		{name: "different origin", subscription: "https://a.example/root", updated: "https://b.example/root/child"},
		{
			name:         "query identity is exact only",
			subscription: "https://a.example/root?v=1",
			updated:      "https://a.example/root/child?v=1",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			matched := resourceSubscriptionAllows([]string{fixture.subscription}, fixture.updated)

			// Assert.
			require.Equal(t, fixture.want, matched)
		})
	}
}

func TestClient_LoggingNotificationAllowsNullData(t *testing.T) {
	// Arrange.
	var output bytes.Buffer
	client := &Client{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	id := json.RawMessage(`2`)
	key, err := rpcIDKey(id)
	require.NoError(t, err)
	client.requestLogs.Store("r:"+key, &struct{}{})

	// Act.
	client.handleRequestLogMessage(id, json.RawMessage(`{"level":"info","data":null}`))

	// Assert.
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "null", record["data"])
}

func TestClient_LoggingNotificationRequiresCapabilityAndPreservesData(t *testing.T) {
	// Arrange.
	var output bytes.Buffer
	client := &Client{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	id := json.RawMessage(`2`)
	key, err := rpcIDKey(id)
	require.NoError(t, err)
	raw := mustJSON(t, LogMessageParams{Level: "info", Logger: "server", Data: json.RawMessage(`{"ok":true}`)})
	client.handleRequestLogMessage(id, raw)
	require.Empty(t, output.String(), "uncorrelated logs must not reach the host logger")
	client.requestLogs.Store("r:"+key, &struct{}{})

	// Act.
	client.handleRequestLogMessage(id, raw)

	// Assert.
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "INFO", record["level"])
	require.Equal(t, "server", record["logger"])
	require.JSONEq(t, `{"ok":true}`, record["data"].(string))
	client.requestLogs.Delete("r:" + key)
	output.Reset()
	client.handleRequestLogMessage(id, raw)
	require.Empty(t, output.String(), "late logs must not reach the host logger")
}

func TestClient_PromptTaggedContentAndMetadata(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Prompts: &PromptsCapability{}}), nil, true
		case MethodPromptsGet:
			result := PromptsGetResult{ResultType: ResultTypeComplete, Messages: []PromptMessage{
				{
					Role: "user",
					Content: ContentBlock{Type: "resource", Resource: &ResourceContents{
						URI: "file:///prompt", Text: new("prompt text"), Meta: Meta{"x": json.RawMessage(`1`)},
					}},
				},
			}}
			return mustJSON(t, result), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	result, err := client.GetPrompt(context.Background(), "prompt", nil)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, "resource", result.Messages[0].Content.Type)
	require.JSONEq(t, `1`, string(result.Messages[0].Content.Resource.Meta["x"]))
}
