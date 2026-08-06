package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRPCPeer_RequestAndNotificationWithSameMethodAreDistinct(t *testing.T) {
	// Arrange.
	var mu sync.Mutex
	var sent [][]byte
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		mu.Lock()
		sent = append(sent, append([]byte(nil), body...))
		mu.Unlock()
		return nil
	})
	notifications := 0
	requests := 0
	peer.setNotificationHandler("same", func(json.RawMessage) { notifications++ })
	peer.setRequestHandler(func(_ context.Context, _ Request) (json.RawMessage, *JSONRPCError) {
		requests++
		return json.RawMessage(`{"ok":true}`), nil
	})

	// Act.
	require.NoError(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","method":"same","params":{}}`)))
	require.NoError(
		t,
		peer.dispatch([]byte(`{"jsonrpc":"2.0","id":"x","method":"same","params":{}}`)),
	)

	// Assert.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(sent) == 1
	}, time.Second, time.Millisecond)
	require.Equal(t, 1, notifications)
	require.Equal(t, 1, requests)
	mu.Lock()
	defer mu.Unlock()
	var response Response
	require.NoError(t, json.Unmarshal(sent[0], &response))
	require.JSONEq(t, `"x"`, string(response.ID))
	require.JSONEq(t, `{"ok":true}`, string(response.Result))
}

func TestRPCPeer_UnknownIncomingMethodReturnsMethodNotFound(t *testing.T) {
	// Arrange.
	responseCh := make(chan Response, 1)
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		var response Response
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		responseCh <- response
		return nil
	})

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":7,"method":"unknown"}`))

	// Assert.
	require.NoError(t, err)
	response := <-responseCh
	require.JSONEq(t, `7`, string(response.ID))
	require.Equal(t, JSONRPCMethodNotFound, response.Error.Code)
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

func TestRPCPeer_EmptyMethodUsesNormalUnknownMethodSemantics(t *testing.T) {
	// Arrange.
	responses := make(chan Response, 1)
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		var response Response
		require.NoError(t, json.Unmarshal(body, &response))
		responses <- response
		return nil
	})

	// Act.
	requestErr := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":7,"method":""}`))
	notificationErr := peer.dispatch([]byte(`{"jsonrpc":"2.0","method":""}`))

	// Assert.
	require.NoError(t, requestErr)
	require.NoError(t, notificationErr)
	response := <-responses
	require.Equal(t, JSONRPCMethodNotFound, response.Error.Code)
	select {
	case extra := <-responses:
		t.Fatalf("unknown notification produced response: %+v", extra)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestRPCPeer_HybridRequestResponseEnvelopeIsRejected(t *testing.T) {
	// Arrange.
	responseCh := make(chan Response, 1)
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		var response Response
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		responseCh <- response
		return nil
	})

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1.5,"method":"roots/list","result":{}}`))

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
	response := <-responseCh
	require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
	require.JSONEq(t, `null`, string(response.ID))
}

func TestRPCPeer_FractionalIncomingRequestIDIsRejectedWithoutDispatch(t *testing.T) {
	// Arrange.
	handled := false
	responses := make(chan Response, 1)
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		var response Response
		require.NoError(t, json.Unmarshal(body, &response))
		responses <- response
		return nil
	})
	peer.setRequestHandler(func(context.Context, Request) (json.RawMessage, *JSONRPCError) {
		handled = true
		return json.RawMessage(`{}`), nil
	})

	// Act.
	err := peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1.5,"method":"roots/list"}`))

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	response := <-responses
	require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
	require.JSONEq(t, `null`, string(response.ID))
	require.False(t, handled)
}

func TestRPCPeer_MalformedIncomingRequestsReceiveInvalidRequest(t *testing.T) {
	fixtures := []string{
		`{"jsonrpc":"1.0","id":7,"method":"roots/list"}`,
		`{"id":"missing-version","method":"roots/list"}`,
		`{"jsonrpc":"2.0","id":9,"method":42}`,
		`{"jsonrpc":"2.0","id":10,"method":null}`,
	}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			// Arrange.
			responses := make(chan Response, 1)
			peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
				var response Response
				require.NoError(t, json.Unmarshal(body, &response))
				responses <- response
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			response := <-responses
			require.Equal(t, JSONRPCInvalidRequest, response.Error.Code)
		})
	}
}

func TestRPCPeer_JSONRPCErrorCodePreservesExactIntegers(t *testing.T) {
	for _, code := range []string{"1.0", "1e0", "9007199254740993"} {
		t.Run(code, func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("error", nil)
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
	pending, _, err := peer.beginRequest("error", nil)
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

func TestRPCPeer_NonObjectAndInvalidJSONReceiveProtocolErrors(t *testing.T) {
	fixtures := []struct {
		name string
		raw  string
		code JSONNumber
	}{
		{name: "array", raw: `[]`, code: JSONRPCInvalidRequest},
		{name: "null", raw: `null`, code: JSONRPCInvalidRequest},
		{name: "syntax", raw: `{`, code: JSONRPCParseError},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			responses := make(chan Response, 1)
			peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
				var response Response
				require.NoError(t, json.Unmarshal(body, &response))
				responses <- response
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture.raw))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			response := <-responses
			require.Equal(t, fixture.code, response.Error.Code)
			require.JSONEq(t, `null`, string(response.ID))
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
			responses := make(chan []byte, 1)
			peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
				responses <- bytes.Clone(body)
				return nil
			})

			// Act.
			err := peer.dispatch([]byte(fixture))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			select {
			case response := <-responses:
				t.Fatalf("malformed notification produced response: %s", response)
			case <-time.After(25 * time.Millisecond):
			}
		})
	}
}

func TestRPCPeer_CloseCancelsAndWaitsForIncomingRequestHandlers(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	peer.setRequestHandler(func(ctx context.Context, _ Request) (json.RawMessage, *JSONRPCError) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return json.RawMessage(`{}`), nil
	})
	require.NoError(t, peer.dispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"roots/list"}`)))
	<-started
	closeDone := make(chan struct{})

	// Act.
	go func() {
		peer.close(ErrTransportClosed)
		close(closeDone)
	}()
	<-cancelled

	// Assert.
	select {
	case <-closeDone:
		t.Fatal("peer close returned before incoming handler cleanup")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("peer close did not return after incoming handler completed")
	}
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
			responses := make(chan []byte, 1)
			peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
				responses <- bytes.Clone(body)
				return nil
			})

			// Act.
			err := peer.dispatch(fmt.Appendf(nil,
				`{"jsonrpc":"2.0","id":7,"method":"roots/list","params":%s}`,
				params,
			))

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			select {
			case response := <-responses:
				require.Contains(t, string(response), `"code":-32600`)
			case <-time.After(time.Second):
				t.Fatal("invalid request response was not sent")
			}
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
		pending, _, err := peer.beginRequest("test", nil)
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
