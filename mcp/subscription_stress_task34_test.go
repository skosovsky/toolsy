package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTask34SubscriptionOverflowAdvancesAuthorityDuringConcurrentClose(t *testing.T) {
	// Arrange.
	const notificationCount = clientEventBufferSize * 4
	listenCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	key, err := rpcIDKey(json.RawMessage(`"sub-overflow"`))
	require.NoError(t, err)
	state := &subscriptionState{
		key:       key,
		publicID:  "sub-overflow",
		requested: SubscriptionFilter{ToolsListChanged: true},
		effective: SubscriptionFilter{ToolsListChanged: true},
		acked:     true,
		events:    make(chan Invalidation, clientEventBufferSize),
		done:      make(chan struct{}),
		cancel:    cancel,
	}
	client := &Client{
		transport:     newContractTransport(func(string, json.RawMessage) (json.RawMessage, error) { return nil, nil }),
		logger:        slog.New(slog.DiscardHandler),
		subscriptions: map[string]*subscriptionState{key: state},
		toolBindings:  make(map[string]struct{}),
		invalidations: make(chan Invalidation, clientEventBufferSize),
	}
	subscription := &Subscription{ID: state.publicID, Events: state.events, cancel: cancel}
	raw := json.RawMessage(`{"_meta":{"io.modelcontextprotocol/subscriptionId":"sub-overflow"}}`)
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(9)

	// Act.
	go func() {
		defer workers.Done()
		<-start
		for range notificationCount {
			client.handleSubscriptionInvalidation(InvalidationTools, raw)
		}
	}()
	for range 8 {
		go func() {
			defer workers.Done()
			<-start
			for range 128 {
				subscription.Close()
			}
		}()
	}
	close(start)
	workers.Wait()

	// Assert.
	require.Equal(t, uint64(notificationCount), client.InvalidationGeneration())
	require.Equal(t, uint64(notificationCount), client.DiscoveryGeneration(InvalidationTools))
	require.Len(t, state.events, clientEventBufferSize)
	require.Len(t, client.invalidations, clientEventBufferSize)
	require.ErrorIs(t, listenCtx.Err(), context.Canceled)
}
