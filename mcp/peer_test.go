package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRPCPeer_RequestAndNotificationWithSameMethodAreDistinct(t *testing.T) {
	// Arrange.
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})
	notifications := 0
	peer.setNotificationHandler("same", func(json.RawMessage) { notifications++ })

	// Act.
	require.NoError(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","method":"same","params":{}}`)))
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":"x","method":"same","params":{}}`))

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, 1, notifications)
	require.Zero(t, sent, "client must not answer a server-initiated request")
}

func TestRPCPeer_UnknownIncomingMethodFailsClosed(t *testing.T) {
	// Arrange.
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":7,"method":"unknown"}`))

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.Zero(t, sent)
}

func TestRPCPeer_StringAndNumericIDsDoNotCollide(t *testing.T) {
	// Arrange.
	require.NotEqual(t, mustIDKey(t, `1`), mustIDKey(t, `"1"`))

	// Act.
	_, fractionalErr := rpcIDKey(json.RawMessage(`1.5`))
	_, nullErr := rpcIDKey(json.RawMessage(`null`))

	// Assert.
	require.Error(t, fractionalErr)
	require.Error(t, nullErr)
	require.Equal(t, "n:900719925474099312345", mustIDKey(t, `900719925474099312345`))
	require.Equal(t, mustIDKey(t, `1`), mustIDKey(t, `1.0`))
	require.Equal(t, mustIDKey(t, `1`), mustIDKey(t, `1e0`))
	require.Equal(t, mustIDKey(t, `1000`), mustIDKey(t, `1e3`))
	require.Equal(t, "n:1e1000000", mustIDKey(t, `1e1000000`))
}

func TestRPCPeer_EmptyMethodFailsClosed(t *testing.T) {
	// Arrange.
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})

	// Act.
	requestErr := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":7,"method":""}`))
	notificationErr := peer.dispatch([]byte(`{"jsonrpc":"2.0","method":""}`))

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, requestErr, &invalid)
	require.ErrorAs(t, notificationErr, &invalid)
	require.Zero(t, sent)
}

func TestRPCPeer_HybridRequestResponseEnvelopeIsRejected(t *testing.T) {
	// Arrange.
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1.5,"method":"roots/list","result":{}}`))

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
	require.Zero(t, sent)
}

func TestRPCPeer_FractionalIncomingRequestIDIsRejectedWithoutDispatch(t *testing.T) {
	// Arrange.
	handled := 0
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})
	peer.setNotificationHandler("roots/list", func(json.RawMessage) { handled++ })

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1.5,"method":"roots/list"}`))

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.Zero(t, sent)
	require.Zero(t, handled)
}

func TestRPCPeer_MalformedIncomingRequestsFailClosed(t *testing.T) {
	fixtures := []string{
		`{"jsonrpc":"1.0","id":7,"method":"roots/list"}`,
		`{"id":"missing-version","method":"roots/list"}`,
		`{"jsonrpc":"2.0","id":9,"method":42}`,
		`{"jsonrpc":"2.0","id":10,"method":null}`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			sent := 0
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
				sent++
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.Zero(t, sent)
		})
	}
}

func TestRPCPeer_JSONRPCErrorCodePreservesExactIntegers(t *testing.T) {
	for _, code := range []string{"1.0", "1e0", "9007199254740993"} {
		t.Run(code, func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("error", struct{}{})
			require.NoError(t, err)
			response := fmt.Appendf(
				nil,
				`{"jsonrpc":"2.0","id":%s,"error":{"code":%s,"message":"failure"}}`,
				pending.ID(), code,
			)

			// Act.
			dispatchErr := peer.dispatch(response)
			_, awaitErr := pending.Await(context.Background())

			// Assert.
			require.NoError(t, dispatchErr)
			var rpcErr *RPCError
			require.ErrorAs(t, awaitErr, &rpcErr)
			require.Equal(t, JSONNumber(code), rpcErr.Code)
		})
	}
}

func TestRPCPeer_JSONRPCErrorCodeRejectsFraction(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest("error", struct{}{})
	require.NoError(t, err)
	response := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":%s,"error":{"code":1.5,"message":"failure"}}`,
		pending.ID(),
	)

	// Act.
	dispatchErr := peer.dispatch(response)
	_, awaitErr := pending.Await(context.Background())

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, dispatchErr, &invalid)
	require.ErrorAs(t, awaitErr, &invalid)
}

func TestRPCPeer_NonObjectAndInvalidJSONFailClosed(t *testing.T) {
	fixtures := []struct {
		name string
		raw  string
	}{
		{name: "array", raw: `[]`},
		{name: "null", raw: `null`},
		{name: "syntax", raw: `{`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			sent := 0
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
				sent++
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture.raw))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.Zero(t, sent)
		})
	}
}

func TestRPCPeer_MalformedNotificationShapedMessagesNeverReceiveResponse(t *testing.T) {
	fixtures := []string{
		`{"jsonrpc":"2.0","method":"notifications/progress","params":[]}`,
		`{"jsonrpc":"1.0","method":"notifications/progress","params":{}}`,
		`{"jsonrpc":"2.0","method":42,"params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/progress","result":{}}`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			sent := 0
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
				sent++
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.Zero(t, sent)
		})
	}
}

func TestRPCPeer_CloseAfterForbiddenIncomingRequest(t *testing.T) {
	// Arrange.
	sent := 0
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
		sent++
		return nil
	})
	var invalid *InvalidPayloadError
	require.ErrorAs(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"roots/list"}`)), &invalid)

	// Act.
	peer.close(ErrTransportClosed)

	// Assert.
	require.Zero(t, sent)
	_, _, err := peer.beginRequest("after-close", nil)
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestRPCPeer_InvalidErrorObjectCompletesCorrelatedRequest(t *testing.T) {
	fixtures := []string{`{}`, `{"code":null,"message":"x"}`, `{"code":1,"message":null}`}
	for _, errorObject := range fixtures {
		t.Run(errorObject, func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("invalid-error", struct{}{})
			require.NoError(t, err)
			payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":%s}`, pending.ID(), errorObject)

			// Act.
			dispatchErr := peer.dispatch([]byte(payload))
			_, awaitErr := pending.Await(context.Background())

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, dispatchErr, &invalid)
			require.ErrorAs(t, awaitErr, &invalid)
		})
	}
}

func TestRPCPeer_RejectsNonObjectParams(t *testing.T) {
	fixtures := []string{"null", `[]`, `"scalar"`, `1`}
	for _, params := range fixtures {
		t.Run(params, func(t *testing.T) {
			// Arrange.
			sent := 0
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error {
				sent++
				return nil
			})
			handled := 0
			peer.setNotificationHandler("same", func(json.RawMessage) { handled++ })

			// Act.
			err := peer.dispatch(fmt.Appendf(nil,
				`{"jsonrpc":"2.0","id":7,"method":"roots/list","params":%s}`,
				params,
			))
			notificationErr := peer.dispatch(fmt.Appendf(nil,
				`{"jsonrpc":"2.0","method":"same","params":%s}`, params,
			))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.ErrorAs(t, notificationErr, &invalid)
			require.Zero(t, sent)
			require.Zero(t, handled)
		})
	}
}

func TestRPCPeer_CloseUnblocksEveryPendingRequest(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(
		context.Background(),
		nil,
		func(context.Context, []byte) error { return nil },
	)
	requests := make([]*pendingRequest, 10)
	for index := range requests {
		pending, _, err := peer.beginRequest("test", struct{}{})
		require.NoError(t, err)
		requests[index] = pending
	}

	// Act.
	peer.close(ErrTransportClosed)

	// Assert.
	for _, pending := range requests {
		_, err := pending.Await(context.Background())
		require.ErrorIs(t, err, ErrTransportClosed)
	}
	_, _, err := peer.beginRequest("after-close", nil)
	require.ErrorIs(t, err, ErrTransportClosed)
}

func mustIDKey(t *testing.T, raw string) string {
	t.Helper()
	key, err := rpcIDKey(json.RawMessage(raw))
	require.NoError(t, err)
	return key
}
