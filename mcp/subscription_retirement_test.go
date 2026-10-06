package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func retirementClient(t *testing.T, opts ...ClientOption) (*Client, *fakeTransport) {
	t.Helper()
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method == MethodServerDiscover {
			return completeDiscovery(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}), nil, true
		}
		if request.Method == MethodSubscriptionsListen {
			require.NoError(t, transport.emit(MethodSubscriptionsAcknowledged, map[string]any{
				"notifications": SubscriptionFilter{
					ToolsListChanged: true,
				}, "_meta": Meta{metaSubscriptionID: request.ID},
			}))
			return nil, nil, false
		}
		return nil, nil, true
	}
	client, err := Connect(t.Context(), transport, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client, transport
}

func TestCanceledSubscriptionLateMessagesLeaveSiblingLive(t *testing.T) {
	for _, closeParent := range []bool{false, true} {
		t.Run(boolSubscriptionCase(closeParent), func(t *testing.T) {
			// Arrange: two independent live subscriptions with acknowledged filters.
			client, transport := retirementClient(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a, err := client.Listen(ctx, SubscriptionFilter{ToolsListChanged: true})
			require.NoError(t, err)
			b, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
			require.NoError(t, err)
			defer b.Close()
			// Act: local cancellation retires A before late traffic is processed.
			if closeParent {
				cancel()
			} else {
				a.Close()
			}
			select {
			case <-a.Done():
			case <-time.After(time.Second):
				t.Fatal("A did not close")
			}
			for range 5 {
				require.NoError(t, transport.emit(MethodSubscriptionsAcknowledged, map[string]any{
					"notifications": SubscriptionFilter{
						ToolsListChanged: true,
					}, "_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)},
				}))
				require.NoError(
					t,
					transport.emit(
						MethodToolsListChanged,
						map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)}},
					),
				)
			}
			// Assert: no generation drift or failure of B from retired traffic.
			require.Zero(t, client.DiscoveryGeneration(InvalidationTools))
			select {
			case <-b.Done():
				t.Fatalf("B failed: %v", b.Err())
			default:
			}
			require.NoError(
				t,
				transport.emit(
					MethodToolsListChanged,
					map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(b.ID)}},
				),
			)
			select {
			case event := <-b.Events:
				require.Equal(t, b.ID, event.SubscriptionID)
			case <-time.After(time.Second):
				t.Fatal("B stopped receiving")
			}
			require.Equal(t, uint64(1), client.DiscoveryGeneration(InvalidationTools))
		})
	}
}

func boolSubscriptionCase(parent bool) string {
	if parent {
		return "parent-cancel"
	}
	return "close"
}

func TestSubscriptionRetirementSaturationAndExpiry(t *testing.T) {
	// Arrange: reserve one retired route alongside one active sibling.
	client, transport := retirementClient(
		t,
		WithSubscriptionLimits(SubscriptionLimits{MaxActive: 2, MaxTracked: 2, RetireTTL: time.Minute}),
	)
	a, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	b, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	defer b.Close()
	a.Close()
	<-a.Done()
	// Act: new admission must not evict A to satisfy the memory bound.
	refused, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	// Assert.
	require.Nil(t, refused)
	require.ErrorContains(t, err, "correlation capacity")
	require.NoError(
		t,
		transport.emit(
			MethodToolsListChanged,
			map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)}},
		),
	)
	select {
	case <-b.Done():
		t.Fatal("capacity refusal disrupted B")
	default:
	}
	client.mu.Lock()
	require.Len(t, client.retiredSubscriptions, 1)
	for key := range client.retiredSubscriptions {
		client.retiredSubscriptions[key] = time.Now().Add(-time.Second)
	}
	client.mu.Unlock()
	// Act: beyond the explicit window A is unknown, preserving strict handling.
	require.NoError(
		t,
		transport.emit(
			MethodToolsListChanged,
			map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)}},
		),
	)
	select {
	case <-b.Done():
		require.ErrorContains(t, b.Err(), "unknown ID")
	case <-time.After(time.Second):
		t.Fatal("unknown-ID protocol violation ignored")
	}
}

func TestRetiredSubscriptionMalformedResourceStillFailsStrictly(t *testing.T) {
	// Arrange: correlation is retired, but malformed protocol payload is not valid late traffic.
	client, transport := retirementClient(t)
	a, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	b, err := client.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	defer b.Close()
	a.Close()
	<-a.Done()
	// Act.
	require.NoError(t, transport.emit(MethodResourceUpdated, map[string]any{
		"uri": "", "_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)},
	}))
	// Assert: unknown/malformed protocol remains strict, without generation mutation.
	select {
	case <-b.Done():
		require.ErrorContains(t, b.Err(), "invalid retired resource")
	case <-time.After(time.Second):
		t.Fatal("malformed retired notification was ignored")
	}
	require.Zero(t, client.DiscoveryGeneration(InvalidationTools))
}

func TestCancelInProgressStillValidatesMalformedResource(t *testing.T) {
	// Arrange: pause the advisory cancellation notification while A is locally canceled
	// but before its await goroutine transfers the route to the retirement map.
	c, tr := retirementClient(t)
	a, err := c.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	b, err := c.Listen(t.Context(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	defer b.Close()
	entered, resume := make(chan struct{}), make(chan struct{})
	tr.mu.Lock()
	tr.notifyHook = func(n capturedNotification) {
		if n.Method != MethodCancelled {
			return
		}
		var params CancelledParams
		_ = json.Unmarshal(n.Params, &params)
		if string(params.RequestID) == a.ID {
			close(entered)
			<-resume
		}
	}
	tr.mu.Unlock()
	// Act.
	a.Close()
	<-entered
	require.NoError(
		t,
		tr.emit(MethodResourceUpdated, map[string]any{"_meta": Meta{metaSubscriptionID: json.RawMessage(a.ID)}}),
	)
	// Assert: missing mandatory URI must not become valid solely due to cancel timing.
	failed := b.Err() != nil
	close(resume)
	<-a.Done()
	require.True(t, failed, "malformed resource ignored during local-cancel transition")
}
