package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigratedFakePreparationIsInertAndDetached(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	params := migrationDiscoveryParams()
	result := json.RawMessage(`{"value":"original"}`)
	calls := 0
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		calls++
		var wire DiscoverParams
		require.NoError(t, json.Unmarshal(request.Params, &wire))
		require.Equal(t, "migration-test", wire.Meta.ClientInfo.Name)
		request.Params[0] = '!'
		request.ID[0] = '9'
		return result, nil, true
	}

	// Act.
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, params)
	require.NoError(t, err)
	params.Meta.ClientInfo.Name = "mutated"
	id := pending.ID()
	id[0] = '9'

	// Assert. Preparation has not executed the hook or sent any bytes.
	require.Zero(t, calls)
	require.Empty(t, transport.requests)
	require.Len(t, transport.preparations, 1)
	require.Equal(t, "1", string(pending.ID()))
	delivery := pending.(DeliveryPendingRequest)
	require.False(t, delivery.WasSent())
	select {
	case <-delivery.DeliveryDone():
		t.Fatal("preparation completed delivery")
	default:
	}
	require.NoError(t, pending.Deliver())
	copy(result, []byte(`{"value":"mutated!"}`))
	actual, err := pending.Await(context.Background())
	require.NoError(t, err)
	require.JSONEq(t, `{"value":"original"}`, string(actual))
	require.Equal(t, 1, calls)
	require.True(t, delivery.WasSent())
	require.Len(t, transport.requests, 1)
	require.True(t, json.Valid(transport.requests[0].Params))
	require.Equal(t, "1", string(transport.requests[0].ID))
	require.Error(t, pending.Deliver())
	require.Len(t, transport.requests, 1)
}

func TestMigratedFakeAbortCompletesOnceWithoutTraffic(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	completed := 0
	pending.(CompletionPendingRequest).OnComplete(func() { completed++ })

	// Act.
	require.NoError(t, pending.Abort(context.Canceled))
	require.Error(t, pending.Abort(context.Canceled))
	require.Error(t, pending.Deliver())
	_, err = pending.Await(context.Background())

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, completed)
	require.Empty(t, transport.requests)
	require.False(t, pending.(DeliveryPendingRequest).WasSent())
	pending.(CompletionPendingRequest).OnComplete(func() { completed++ })
	require.Equal(t, 2, completed, "each registered completion hook runs exactly once")
}

func TestMigratedFakeCancellationOwnsOneNotification(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Act.
	require.True(t, pending.(CancellablePendingRequest).CancelPending())
	require.True(t, pending.(CancellablePendingRequest).CancelPending())
	_, err = pending.Await(ctx)

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, pending.(DeliveryPendingRequest).WasSent())
	owner := pending.(CancellationNotificationPending)
	require.True(t, owner.ClaimCancellationNotification())
	require.False(t, owner.ClaimCancellationNotification())
}

func TestMigratedFakeRacingCompletionRunsHookOnce(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	var completed atomic.Int32
	pending.(CompletionPendingRequest).OnComplete(func() { completed.Add(1) })
	var wait sync.WaitGroup

	// Act.
	for range 64 {
		wait.Go(func() {
			transport.finish(pending.(*preparedRequest).pendingRequest, json.RawMessage(`{}`), nil)
		})
	}
	wait.Wait()
	result, err := pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
	require.EqualValues(t, 1, completed.Load())
}

func TestMigratedFakeCloseUnblocksPreparedAndDelivered(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	require.NoError(t, transport.Start(context.Background()))
	prepared, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	delivered, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, delivered.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Act.
	require.NoError(t, transport.Close())

	// Assert.
	for _, pending := range []PreparedRequest{prepared, delivered} {
		result, err := pending.Await(ctx)
		require.ErrorIs(t, err, ErrTransportClosed)
		require.Empty(t, bytes.TrimSpace(result))
	}
	require.False(t, prepared.(DeliveryPendingRequest).WasSent())
	require.True(t, delivered.(DeliveryPendingRequest).WasSent())
}
