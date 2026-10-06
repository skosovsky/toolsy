package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestClient_NonToolCancellationUsesActiveRequestID(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		}
		return nil, nil, false
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		for _, iterErr := range client.Discover(ctx) {
			done <- iterErr
			return
		}
	}()
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.requests) == 2
	}, time.Second, time.Millisecond)

	// Act.
	cancel()
	err = <-done

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.notifications) == 1
	}, time.Second, time.Millisecond)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var cancellations int
	for _, notification := range transport.notifications {
		if notification.Method == MethodCancelled {
			cancellations++
			var params CancelledParams
			require.NoError(t, json.Unmarshal(notification.Params, &params))
			require.JSONEq(t, string(transport.requests[1].ID), string(params.RequestID))
		}
	}
	require.Equal(t, 1, cancellations)
}

func TestClient_ToolsInvalidationAbortsInFlightSnapshot(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	var listenID json.RawMessage
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodServerDiscover:
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}), nil, true
		case MethodSubscriptionsListen:
			listenID = request.ID
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
		case MethodToolsList:
			require.NoError(
				t,
				transport.emit(MethodToolsListChanged, map[string]any{"_meta": Meta{metaSubscriptionID: listenID}}),
			)
			return mustJSON(
				t,
				ToolsListResult{
					ResultType: ResultTypeComplete,
					TTLMS:      JSONNumber("0"), CacheScope: CacheScopePrivate,
					Tools: []MCPTool{{
						Name:        "stale",
						InputSchema: json.RawMessage(`{"type":"object"}`),
					}},
				},
			), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, client.Close()) })
	subscription, err := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	defer subscription.Close()

	// Act.
	var discoveryErr error
	for _, iterErr := range client.Discover(context.Background()) {
		discoveryErr = iterErr
	}

	// Assert.
	var staleErr *StaleDiscoveryError
	require.ErrorAs(t, discoveryErr, &staleErr)
}

func TestClient_TerminalYieldAbortDoesNotSendObsoleteCancellation(t *testing.T) {
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
						{Name: "abort", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		case MethodToolsCall:
			return mustJSON(
				t,
				CallToolResult{ResultType: ResultTypeComplete, Content: []ContentBlock{{Type: "text", Text: "done"}}},
			), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	var tool toolsy.Tool
	for discovered, iterErr := range client.Discover(context.Background()) {
		require.NoError(t, iterErr)
		tool = discovered
	}

	// Act.
	require.NotNil(t, tool)
	stop := errors.New("stop")
	err = tool.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error {
			return stop
		},
	)

	// Assert.
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	require.ErrorIs(t, err, stop)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var cancellations int
	for _, notification := range transport.notifications {
		if notification.Method == MethodCancelled {
			cancellations++
		}
	}
	require.Zero(t, cancellations, "terminal response already owns completion; cancellation must not follow it")
}
