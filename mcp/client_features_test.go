package mcp

import (
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
		server: InitializeResult{Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: true},
		}},
		initialized: true,
	}

	// Act.
	client.handleListChanged(InvalidationTools, json.RawMessage(`{"_meta":null}`))

	// Assert.
	require.Zero(t, client.toolGeneration.Load())
	require.Empty(t, client.invalidations)
}

func TestClient_SubscribeResourceRequiresObjectResultBeforeMutation(t *testing.T) {
	for _, fixture := range []string{"null", `[]`, `42`, `"scalar"`} {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			transport := newFakeTransport()
			transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
				if request.Method == MethodInitialize {
					return initializeResult(ServerCapabilities{
						Resources: &ResourcesCapability{Subscribe: true},
					}), nil, true
				}
				return json.RawMessage(fixture), nil, true
			}
			client, err := Connect(context.Background(), transport)
			require.NoError(t, err)

			// Act.
			err = client.SubscribeResource(context.Background(), "file:///resource")

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			client.mu.RLock()
			_, subscribed := client.subscribed["file:///resource"]
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
		if request.Method == MethodInitialize {
			return initializeResult(ServerCapabilities{
				Resources: &ResourcesCapability{Subscribe: true},
			}), nil, true
		}
		return json.RawMessage(`{"_meta":{"trace":"x"},"extension":{"ok":true}}`), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	err = client.SubscribeResource(context.Background(), "file:///resource")

	// Assert.
	require.NoError(t, err)
	client.mu.RLock()
	_, subscribed := client.subscribed["file:///resource"]
	client.mu.RUnlock()
	require.True(t, subscribed)
	require.NoError(t, client.Close())
}

func TestClient_CloseIsConcurrentIdempotentAndClosesEventChannels(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{}), nil, true
	}
	client, err := Connect(context.Background(), transport)
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
	_, logMessagesOpen := <-client.LogMessages()
	require.False(t, invalidationsOpen)
	require.False(t, logMessagesOpen)
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
				Execution:   &ToolExecution{TaskSupport: "required"},
			},
			as: new(*UnsupportedFeatureError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			_, err := client.toolToProxy(tc.tool)

			// Assert.
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
		case MethodInitialize:
			return initializeResult(
				ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}},
			), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					Tools: []MCPTool{
						{Name: "stale", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		default:
			return nil, errors.New("stale proxy must not call transport"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var proxy toolsy.Tool
	for item, iterErr := range client.GetTools(context.Background()) {
		require.NoError(t, iterErr)
		proxy = item
	}

	// Act.
	transport.emit(MethodToolsListChanged, struct{}{})
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
		return initializeResult(
			ServerCapabilities{Tools: &ToolsCapability{ListChanged: false}},
		), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	transport.emit(MethodToolsListChanged, struct{}{})

	// Assert.
	require.Zero(t, client.DiscoveryGeneration(InvalidationTools))
}

func TestClient_ResourceUpdateRequiresNegotiatedSubscription(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(
				ServerCapabilities{Resources: &ResourcesCapability{Subscribe: true}},
			), nil, true
		case MethodResourcesSubscribe:
			return json.RawMessage(`{}`), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	require.NoError(t, client.SubscribeResource(context.Background(), "file:///watched"))

	// Act.
	transport.emit(MethodResourceUpdated, ResourceUpdatedParams{URI: "file:///watched"})

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
		case MethodInitialize:
			return initializeResult(
				ServerCapabilities{Resources: &ResourcesCapability{Subscribe: true}},
			), nil, true
		case MethodResourcesSubscribe:
			return json.RawMessage(`{}`), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	require.NoError(t, client.SubscribeResource(context.Background(), "file:///watched"))

	// Act.
	transport.emit(MethodResourceUpdated, ResourceUpdatedParams{URI: "file:///watched/child"})

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
			matched := resourceUpdateMatchesSubscription(fixture.subscription, fixture.updated)

			// Assert.
			require.Equal(t, fixture.want, matched)
		})
	}
}

func TestClient_LoggingNotificationAllowsNullData(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{Logging: json.RawMessage(`{}`)}), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	transport.emit(MethodLogMessage, json.RawMessage(`{"level":"info","data":null}`))

	// Assert.
	select {
	case message := <-client.LogMessages():
		require.JSONEq(t, `null`, string(message.Data))
	case <-time.After(time.Second):
		t.Fatal("missing logging notification with null data")
	}
}

func TestClient_LoggingNotificationRequiresCapabilityAndPreservesData(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(capturedRequest) (json.RawMessage, error, bool) {
		return initializeResult(ServerCapabilities{Logging: json.RawMessage(`{}`)}), nil, true
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	transport.emit(
		MethodLogMessage,
		LogMessageParams{Level: "info", Logger: "server", Data: json.RawMessage(`{"ok":true}`)},
	)

	// Assert.
	select {
	case message := <-client.LogMessages():
		require.Equal(t, "info", message.Level)
		require.JSONEq(t, `{"ok":true}`, string(message.Data))
	case <-time.After(time.Second):
		t.Fatal("missing log message")
	}
}

func TestClient_PromptTaggedContentAndMetadata(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Prompts: &PromptsCapability{}}), nil, true
		case MethodPromptsGet:
			result := PromptsGetResult{Messages: []PromptMessage{
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
