package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRPCPeer_BeginRequestCloseRaceNeverLeaks(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(
		context.Background(),
		nil,
		func(context.Context, []byte) error { return nil },
	)
	peer.limits.MaxInFlight = 512 // This fixture isolates shutdown races, not admission limits.
	var wait sync.WaitGroup
	errorsCh := make(chan error, 512)

	// Act.
	for range 512 {
		wait.Go(func() {
			pending, _, err := peer.beginRequest("race", struct{}{})
			if err != nil {
				errorsCh <- err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = pending.Await(ctx)
			errorsCh <- err
		})
	}
	peer.close(ErrTransportClosed)
	wait.Wait()
	close(errorsCh)

	// Assert.
	for err := range errorsCh {
		require.ErrorIs(t, err, ErrTransportClosed)
	}
}

func TestRPCPeer_IncomingRequestsAreForbiddenUnderConcurrentLoad(t *testing.T) {
	// Arrange: no server handlers or replies may be created at any load.
	var replies atomic.Int32
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		replies.Add(1)
		return nil
	})
	var wait sync.WaitGroup
	errs := make(chan error, 513)
	// Act.
	for id := 1; id <= 513; id++ {
		wait.Go(func() {
			errs <- peer.dispatch(fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`, id))
		})
	}
	wait.Wait()
	close(errs)
	// Assert.
	for err := range errs {
		var invalid *InvalidPayloadError
		require.ErrorAs(t, err, &invalid)
		require.ErrorContains(t, err, "server-initiated requests are not supported")
	}
	require.Zero(t, replies.Load())
}

func TestRPCPeer_RejectsOversizedRequestIDBeforeNumericCanonicalization(t *testing.T) {
	// Arrange.
	oversizedExponent := `1e` + strings.Repeat("9", maxRPCIDBytes)

	// Act.
	_, err := rpcIDKey(json.RawMessage(oversizedExponent))

	// Assert.
	require.Error(t, err)
}

func TestRPCPeer_ResponseCancellationRaceConsumesOneTerminalResponse(t *testing.T) {
	for range 256 {
		// Arrange.
		peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
		pending, _, err := peer.beginRequest("race", struct{}{})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID())
		start := make(chan struct{})
		awaitResult := make(chan error, 1)
		dispatchResult := make(chan error, 1)

		// Act.
		go func() {
			<-start
			_, awaitErr := pending.Await(ctx)
			awaitResult <- awaitErr
		}()
		go func() {
			<-start
			dispatchResult <- peer.dispatch(response)
		}()
		close(start)
		awaitErr := <-awaitResult
		dispatchErr := <-dispatchResult
		duplicateErr := peer.dispatch(response)

		// Assert.
		require.NoError(t, dispatchErr)
		if awaitErr != nil {
			require.ErrorIs(t, awaitErr, context.Canceled)
		}
		require.Error(t, duplicateErr)
	}
}

func TestRPCPeer_ExternalCancellationUnblocksBackgroundAwait(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest("cancel", struct{}{})
	require.NoError(t, err)
	awaitResult := make(chan error, 1)
	go func() {
		_, awaitErr := pending.Await(context.Background())
		awaitResult <- awaitErr
	}()

	// Act.
	cancelled := pending.CancelPending()

	// Assert.
	require.True(t, cancelled)
	select {
	case awaitErr := <-awaitResult:
		require.ErrorIs(t, awaitErr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("external cancellation did not unblock Await")
	}
}

func TestRPCPeer_CancelledResponseBookkeepingRemainsBounded(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	requestIDs := make([]json.RawMessage, 0, maxCancelledResponseRanges+16)

	// Act.
	for range maxCancelledResponseRanges + 16 {
		pending, _, err := peer.beginRequest("cancel", struct{}{})
		require.NoError(t, err)
		require.True(t, pending.CancelPending())
		requestIDs = append(requestIDs, pending.ID())
	}

	// Assert.
	peer.pendingMu.Lock()
	require.Equal(t, []rpcIDRange{{first: 1, last: maxCancelledResponseRanges + 16}}, peer.cancelledRanges)
	peer.pendingMu.Unlock()
	retainedResponse := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":%s,"result":{}}`,
		requestIDs[len(requestIDs)-1],
	)
	require.NoError(t, peer.dispatch(retainedResponse))
	peer.pendingMu.Lock()
	require.Equal(t, []rpcIDRange{{first: 1, last: maxCancelledResponseRanges + 15}}, peer.cancelledRanges)
	peer.pendingMu.Unlock()

	// The long-lived peer remains usable after more cancellations than the
	// bounded range count because contiguous IDs are represented exactly.
	pending, _, err := peer.beginRequest("cancel", struct{}{})
	require.NoError(t, err)
	require.True(t, pending.CancelPending())
	require.False(t, peer.closed.Load())
	after, _, err := peer.beginRequest("after-storm", struct{}{})
	require.NoError(t, err)
	peer.failPending(after, ErrTransportClosed)
}

func TestRPCPeer_CancellationStormNeverMasksDuplicateCompletedResponse(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	completed, _, err := peer.beginRequest("completed", struct{}{})
	require.NoError(t, err)
	completedResponse := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":%s,"result":{}}`,
		completed.ID(),
	)
	require.NoError(t, peer.dispatch(completedResponse))
	for range maxCancelledResponseRanges + 16 {
		pending, _, beginErr := peer.beginRequest("cancel", struct{}{})
		require.NoError(t, beginErr)
		require.True(t, pending.CancelPending())
	}

	// Act.
	err = peer.dispatch(completedResponse)

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
}

func TestRPCPeer_OutOfOrderCancellationsRemainExactlyCorrelated(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	first, _, err := peer.beginRequest("cancel", struct{}{})
	require.NoError(t, err)
	second, _, err := peer.beginRequest("cancel", struct{}{})
	require.NoError(t, err)

	// Act.
	require.True(t, second.CancelPending())
	require.True(t, first.CancelPending())
	firstErr := peer.dispatch(fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":{}}`, first.ID()))
	secondErr := peer.dispatch(fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":{}}`, second.ID()))

	// Assert.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.Empty(t, peer.cancelledRanges)
}

func TestRPCPeer_CancellationRangeInsertionAndBridgingStaySorted(t *testing.T) {
	sequences := [][]uint64{{3, 1, 2}, {2, 3, 1}, {3, 2, 1}}
	for _, sequence := range sequences {
		t.Run(fmt.Sprint(sequence), func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })

			// Act.
			for _, id := range sequence {
				require.True(t, peer.addCancelledLocked(id))
			}

			// Assert.
			require.Equal(t, []rpcIDRange{{first: 1, last: 3}}, peer.cancelledRanges)
		})
	}
}

func TestRPCPeer_CancellationRangeFragmentationFailsClosedAtBound(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	for range maxCancelledResponseRanges {
		cancelled, _, err := peer.beginRequest("cancel", struct{}{})
		require.NoError(t, err)
		require.True(t, cancelled.CancelPending())
		completed, _, err := peer.beginRequest("complete", struct{}{})
		require.NoError(t, err)
		response := fmt.Appendf(
			nil,
			`{"jsonrpc":"2.0","id":%s,"result":{}}`,
			completed.ID(),
		)
		require.NoError(t, peer.dispatch(response))
	}
	overflow, _, err := peer.beginRequest("overflow", struct{}{})
	require.NoError(t, err)

	// Act.
	require.True(t, overflow.CancelPending())

	// Assert.
	require.True(t, peer.closed.Load())
	_, _, err = peer.beginRequest("after-overflow", struct{}{})
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestRPCPeer_LateCancelledResponseSplitsRangeAndDuplicateFailsClosed(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	ids := make([]json.RawMessage, 0, 3)
	for range 3 {
		pending, _, err := peer.beginRequest("cancel", struct{}{})
		require.NoError(t, err)
		ids = append(ids, pending.ID())
		require.True(t, pending.CancelPending())
	}
	middleResponse := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":%s,"result":{}}`,
		ids[1],
	)

	// Act.
	firstErr := peer.dispatch(middleResponse)
	duplicateErr := peer.dispatch(middleResponse)

	// Assert.
	require.NoError(t, firstErr)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, duplicateErr, &invalid)
	peer.pendingMu.Lock()
	require.Equal(t, []rpcIDRange{{first: 1, last: 1}, {first: 3, last: 3}}, peer.cancelledRanges)
	peer.pendingMu.Unlock()
}

func TestRPCPeer_DuplicateResponseFailsClosedAndNotifyAfterCloseStops(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(
		context.Background(),
		nil,
		func(context.Context, []byte) error { return nil },
	)
	pending, _, err := peer.beginRequest("once", struct{}{})
	require.NoError(t, err)
	response := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID())

	// Act.
	require.NoError(t, peer.dispatch([]byte(response)))
	duplicateErr := peer.dispatch([]byte(response))
	peer.close(ErrTransportClosed)
	notifyErr := peer.notifyMessage(context.Background(), "after", nil)

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, duplicateErr, &payloadErr)
	require.ErrorIs(t, notifyErr, ErrTransportClosed)
}
