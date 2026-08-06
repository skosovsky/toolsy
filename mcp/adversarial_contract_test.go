package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestRPCPeer_BeginRequestCloseRaceNeverLeaks(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(
		context.Background(),
		nil,
		func(context.Context, []byte) error { return nil },
	)
	var wait sync.WaitGroup
	errorsCh := make(chan error, 512)

	// Act.
	for range 512 {
		wait.Go(func() {
			pending, _, err := peer.beginRequest("race", nil)
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

func TestRPCPeer_IncomingRequestConcurrencyIsBounded(t *testing.T) {
	// Arrange.
	release := make(chan struct{})
	started := atomic.Int32{}
	responses := make(chan Response, maxConcurrentIncoming+1)
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		var response Response
		require.NoError(t, json.Unmarshal(body, &response))
		responses <- response
		return nil
	})
	peer.setRequestHandler(func(context.Context, Request) (json.RawMessage, *JSONRPCError) {
		started.Add(1)
		<-release
		return json.RawMessage(`{}`), nil
	})
	for id := 1; id <= maxConcurrentIncoming; id++ {
		require.NoError(t, peer.dispatch(fmt.Appendf(
			nil, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`, id,
		)))
	}
	require.Eventually(t, func() bool {
		return started.Load() == maxConcurrentIncoming
	}, time.Second, time.Millisecond)

	// Act.
	err := peer.dispatch(fmt.Appendf(
		nil, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`, maxConcurrentIncoming+1,
	))
	overload := <-responses

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, overload.Error)
	require.Equal(t, JSONRPCServerOverloaded, overload.Error.Code)
	close(release)
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
		pending, _, err := peer.beginRequest("race", nil)
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
	pending, _, err := peer.beginRequest("cancel", nil)
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

func TestFakePending_AwaitMatchesProductionCancellationRace(t *testing.T) {
	t.Run("terminal result wins over cancelled context", func(t *testing.T) {
		// Arrange.
		pending := newFakePending(json.RawMessage(`1`))
		pending.markSent()
		pending.OnComplete(nil)
		pending.complete(callResult{result: json.RawMessage(`{"ok":true}`)})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act.
		result, err := pending.Await(ctx)

		// Assert.
		require.NoError(t, err)
		require.JSONEq(t, `{"ok":true}`, string(result))
		require.False(t, pending.CancelPending())
	})

	t.Run("active request cancellation claims terminal state", func(t *testing.T) {
		// Arrange.
		pending := newFakePending(json.RawMessage(`1`))
		pending.markSent()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act.
		result, err := pending.Await(ctx)
		terminal, open := <-pending.ch
		_, openAfterTerminal := <-pending.ch

		// Assert.
		require.Nil(t, result)
		require.ErrorIs(t, err, context.Canceled)
		require.True(t, open)
		require.ErrorIs(t, terminal.err, context.Canceled)
		require.False(t, openAfterTerminal)
		require.True(t, pending.CancelPending())
	})
}

func TestFakePending_ConcurrentTerminalWinnerMatchesProductionState(t *testing.T) {
	// Arrange.
	pending := newFakePending(json.RawMessage(`1`))
	pending.markSent()
	pending.OnComplete(nil)
	var callbackCalls atomic.Int64
	pending.OnComplete(func() { callbackCalls.Add(1) })
	start := make(chan struct{})
	var cancelClaimed bool
	var group sync.WaitGroup
	group.Add(2)

	// Act.
	go func() {
		defer group.Done()
		<-start
		pending.complete(callResult{result: json.RawMessage(`{"winner":"response"}`)})
	}()
	go func() {
		defer group.Done()
		<-start
		cancelClaimed = pending.CancelPending()
	}()
	close(start)
	group.Wait()
	terminal, open := <-pending.ch
	_, openAfterTerminal := <-pending.ch
	pending.mu.Lock()
	cancelWon := pending.cancelled
	pending.mu.Unlock()
	repeatedCancel := pending.CancelPending()
	select {
	case <-pending.DeliveryDone():
	default:
		t.Fatal("delivery state did not reach a terminal boundary")
	}
	pending.OnComplete(func() { callbackCalls.Add(1) })

	// Assert.
	require.True(t, pending.WasSent())
	require.True(t, open)
	require.False(t, openAfterTerminal)
	require.Equal(t, cancelWon, cancelClaimed)
	require.Equal(t, cancelWon, repeatedCancel)
	if cancelWon {
		require.ErrorIs(t, terminal.err, context.Canceled)
		require.Nil(t, terminal.result)
	} else {
		require.NoError(t, terminal.err)
		require.JSONEq(t, `{"winner":"response"}`, string(terminal.result))
	}
	require.EqualValues(t, 2, callbackCalls.Load())
}

func TestRPCPeer_CancelledResponseBookkeepingRemainsBounded(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	requestIDs := make([]json.RawMessage, 0, maxCancelledResponseRanges+16)

	// Act.
	for range maxCancelledResponseRanges + 16 {
		pending, _, err := peer.beginRequest("cancel", nil)
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
	pending, _, err := peer.beginRequest("cancel", nil)
	require.NoError(t, err)
	require.True(t, pending.CancelPending())
	require.False(t, peer.closed.Load())
	after, _, err := peer.beginRequest("after-storm", nil)
	require.NoError(t, err)
	peer.failPending(after, ErrTransportClosed)
}

func TestRPCPeer_CancellationStormNeverMasksDuplicateCompletedResponse(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	completed, _, err := peer.beginRequest("completed", nil)
	require.NoError(t, err)
	completedResponse := fmt.Appendf(
		nil,
		`{"jsonrpc":"2.0","id":%s,"result":{}}`,
		completed.ID(),
	)
	require.NoError(t, peer.dispatch(completedResponse))
	for range maxCancelledResponseRanges + 16 {
		pending, _, beginErr := peer.beginRequest("cancel", nil)
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
	first, _, err := peer.beginRequest("cancel", nil)
	require.NoError(t, err)
	second, _, err := peer.beginRequest("cancel", nil)
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
		cancelled, _, err := peer.beginRequest("cancel", nil)
		require.NoError(t, err)
		require.True(t, cancelled.CancelPending())
		completed, _, err := peer.beginRequest("complete", nil)
		require.NoError(t, err)
		response := fmt.Appendf(
			nil,
			`{"jsonrpc":"2.0","id":%s,"result":{}}`,
			completed.ID(),
		)
		require.NoError(t, peer.dispatch(response))
	}
	overflow, _, err := peer.beginRequest("overflow", nil)
	require.NoError(t, err)

	// Act.
	require.True(t, overflow.CancelPending())

	// Assert.
	require.True(t, peer.closed.Load())
	_, _, err = peer.beginRequest("after-overflow", nil)
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestRPCPeer_LateCancelledResponseSplitsRangeAndDuplicateFailsClosed(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	ids := make([]json.RawMessage, 0, 3)
	for range 3 {
		pending, _, err := peer.beginRequest("cancel", nil)
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

func TestSSERetryAcceptsOnlyASCIIIntegerMilliseconds(t *testing.T) {
	// Arrange.
	tests := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{value: "0", want: httpPollRetryDelay, ok: true},
		{value: "1", want: httpPollRetryDelay, ok: true},
		{value: "120000", want: 2 * time.Minute, ok: true},
		{value: "600000", want: httpMaxRetryDelay, ok: true},
		{value: "+1", ok: false},
		{value: "1.5", ok: false},
		{value: "-1", ok: false},
		{value: " 1", ok: false},
		{value: "18446744073709551615", ok: false},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			// Act.
			got, ok := parseSSERetry(test.value)

			// Assert.
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.want, got)
		})
	}
}

func TestStreamableHTTP_CloseCompletesHeldRequestWithTransportClosed(t *testing.T) {
	// Arrange.
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", eventStreamMediaType)
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		close(received)
		<-request.Context().Done()
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "held/close", struct{}{})
	require.NoError(t, err)
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("server did not receive held request")
	}

	// Act.
	require.NoError(t, transport.Close())
	_, err = pending.Await(context.Background())

	// Assert.
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestStreamableHTTP_TerminalIncomingResponseErrorCannotSelfDeadlock(t *testing.T) {
	// Arrange.
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(received)
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	transport.OnRequest(func(context.Context, Request) (json.RawMessage, *JSONRPCError) {
		return json.RawMessage(`{}`), nil
	})
	require.NoError(t, transport.peer.dispatch(
		[]byte(`{"jsonrpc":"2.0","id":"server-request","method":"roots/list"}`),
	))
	<-received
	require.Eventually(t, func() bool {
		transport.mu.RLock()
		defer transport.mu.RUnlock()
		return transport.terminating
	}, time.Second, time.Millisecond)

	// Act.
	closeDone := make(chan struct{})
	go func() {
		_ = transport.Close()
		close(closeDone)
	}()

	// Assert.
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal response error deadlocked transport close")
	}
}

func TestStreamableHTTP_CancellationFollowsOriginalWireRequest(t *testing.T) {
	// Arrange.
	var mu sync.Mutex
	var methods []string
	originalReceived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var message struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		methods = append(methods, message.Method)
		mu.Unlock()
		if message.Method == "ordered/request" {
			close(originalReceived)
			<-request.Context().Done()
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	client := &Client{transport: transport}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.requestAndAwait(ctx, "ordered/request", struct{}{})
		result <- err
	}()
	<-originalReceived

	// Act.
	cancel()
	err := <-result

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(methods) == 2
	}, time.Second, time.Millisecond)
	mu.Lock()
	require.Equal(t, []string{"ordered/request", MethodCancelled}, methods)
	mu.Unlock()
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_PreCancelledRequestNeverAllocatesOrWritesID(t *testing.T) {
	// Arrange.
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act.
	pending, err := transport.Request(ctx, "never/sent", struct{}{})

	// Assert.
	require.Nil(t, pending)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int64(0), requests.Load())
	require.NoError(t, transport.Close())
}

func TestRPCPeer_DuplicateResponseFailsClosedAndNotifyAfterCloseStops(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(
		context.Background(),
		nil,
		func(context.Context, []byte) error { return nil },
	)
	pending, _, err := peer.beginRequest("once", nil)
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

func TestStreamableHTTP_InitializePOSTSSEDisconnectResumesWithPrimingID(t *testing.T) {
	// Arrange.
	getHeader := make(chan string, 1)
	var getCount int
	var getMu sync.Mutex
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.Method {
			case http.MethodPost:
				writer.Header().Set(mcpSessionIDHeader, "resume-session")
				writer.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(writer, "id: initialize-1\ndata:\nretry: 25\n\n")
			case http.MethodGet:
				getMu.Lock()
				getCount++
				count := getCount
				getMu.Unlock()
				if count > 1 {
					writer.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				getHeader <- request.Header.Get("Last-Event-ID")
				writer.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(
					writer,
					"id: initialize-2\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":%s}\n\n",
					initializeResult(ServerCapabilities{}),
				)
			default:
				writer.WriteHeader(http.StatusNoContent)
			}
		}),
	)
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(
		context.Background(),
		MethodInitialize,
		validInitializeParams(),
	)
	require.NoError(t, err)

	// Act.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := pending.Await(ctx)

	// Assert.
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.Equal(t, "initialize-1", <-getHeader)
	transport.mu.RLock()
	require.Equal(t, httpPollRetryDelay, transport.pollRetryDelay)
	transport.mu.RUnlock()
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_SessionCannotBeReplaced(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport("https://example.invalid/mcp")

	// Act.
	firstErr := transport.acceptSessionHeader("session-a", true)
	replacementErr := transport.acceptSessionHeader("session-b", false)
	lateEstablishErr := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
	).acceptSessionHeader("session-b", false)

	// Assert.
	require.NoError(t, firstErr)
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, replacementErr, &payloadErr)
	require.ErrorAs(t, lateEstablishErr, &payloadErr)
}

func TestStreamableHTTP_DecoratorCannotChangeTargetAndEmptyEventIDResetsResume(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			request.URL.Host = "attacker.invalid"
			return nil
		}),
	)
	transport.lastEventID = "old"
	transport.peer = newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })

	// Act.
	_, decoratorErr := transport.buildRequest(context.Background(), http.MethodGet, nil)
	preservedErr := transport.consumeSSE(context.Background(), strings.NewReader("id:  cursor  \n\n"))
	preservedID := transport.lastEventID
	boundaryErr := transport.consumeSSE(context.Background(), strings.NewReader("id:\n\n"))

	// Assert.
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, decoratorErr, &payloadErr)
	require.ErrorIs(t, preservedErr, errSSEPollingBoundary)
	require.Equal(t, " cursor  ", preservedID)
	require.ErrorIs(t, boundaryErr, errSSEPollingBoundary)
	require.Empty(t, transport.lastEventID)
}

func TestStreamableHTTP_DecoratorCannotOverrideRequestHost(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			request.Host = "internal-vhost"
			return nil
		}),
	)

	// Act.
	request, err := transport.buildRequest(context.Background(), http.MethodGet, nil)

	// Assert.
	require.Nil(t, request)
	var payloadErr *InvalidPayloadError
	require.ErrorAs(t, err, &payloadErr)
}

func TestStreamableHTTP_DecoratorCannotSetBodyContract(t *testing.T) {
	mutations := map[string]func(*http.Request){
		"body": func(request *http.Request) { request.Body = http.NoBody },
		"get body": func(request *http.Request) {
			request.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		},
		"content length":    func(request *http.Request) { request.ContentLength = 1 },
		"transfer encoding": func(request *http.Request) { request.TransferEncoding = []string{"chunked"} },
		"trailer":           func(request *http.Request) { request.Trailer = http.Header{"X-Test": {"value"}} },
		"length header":     func(request *http.Request) { request.Header.Set("Content-Length", "1") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			transport := NewStreamableHTTPTransport(
				"https://example.invalid/mcp",
				WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
					mutate(request)
					return nil
				}),
			)

			// Act.
			request, err := transport.buildRequest(
				context.Background(),
				http.MethodPost,
				strings.NewReader(`{"jsonrpc":"2.0"}`),
			)

			// Assert.
			require.Nil(t, request)
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
		})
	}
}

func TestStreamableHTTP_RequestIsDetachedFromDecoratorPointer(t *testing.T) {
	// Arrange.
	var decorated *http.Request
	transport := NewStreamableHTTPTransport(
		"https://example.invalid/mcp",
		WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			request.Header.Set("Authorization", "Bearer original")
			decorated = request
			return nil
		}),
	)
	request, err := transport.buildRequest(
		context.Background(),
		http.MethodPost,
		strings.NewReader(`{"jsonrpc":"2.0"}`),
	)
	require.NoError(t, err)

	// Act.
	decorated.Header.Set("Authorization", "Bearer forged")
	decorated.URL.Host = "attacker.invalid"
	decorated.Body = http.NoBody
	body, readErr := io.ReadAll(request.Body)

	// Assert.
	require.NoError(t, readErr)
	require.Equal(t, "Bearer original", request.Header.Get("Authorization"))
	require.Equal(t, "example.invalid", request.URL.Host)
	require.JSONEq(t, `{"jsonrpc":"2.0"}`, string(body))
}

func TestStreamableHTTP_RejectsPrivateEndpointAndOversizedBody(t *testing.T) {
	// Arrange.
	private := NewStreamableHTTPTransport("http://127.0.0.1:1/mcp")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":{"padding":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}`)
	}))
	defer server.Close()
	bounded := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
		WithStreamableHTTPMaxStreamBytes(32),
	)
	require.NoError(t, bounded.Start(context.Background()))
	pending, err := bounded.Request(context.Background(), "oversized", nil)
	require.NoError(t, err)

	// Act.
	privateErr := private.Start(context.Background())
	_, bodyErr := pending.Await(context.Background())

	// Assert.
	require.Error(t, privateErr)
	require.Error(t, bodyErr)
	require.NoError(t, bounded.Close())
}

func TestStreamableHTTP_RequestRejects202AndCloseCancelsActivePOST(t *testing.T) {
	t.Run("request rejects 202", func(t *testing.T) {
		// Arrange.
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		pending, err := transport.Request(context.Background(), "must-answer", nil)
		require.NoError(t, err)

		// Act.
		_, err = pending.Await(context.Background())

		// Assert.
		var payloadErr *InvalidPayloadError
		require.ErrorAs(t, err, &payloadErr)
		require.NoError(t, transport.Close())
	})

	t.Run("close cancels post", func(t *testing.T) {
		// Arrange.
		started := make(chan struct{})
		releaseServer := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			select {
			case <-request.Context().Done():
			case <-releaseServer:
			}
		}))
		defer server.Close()
		defer close(releaseServer)
		transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
		require.NoError(t, transport.Start(context.Background()))
		pending, err := transport.Request(context.Background(), "blocked", nil)
		require.NoError(t, err)
		<-started

		// Act.
		require.NoError(t, transport.Close())
		_, pendingErr := pending.Await(context.Background())

		// Assert.
		require.Error(t, pendingErr)
		transport.mu.RLock()
		activePosts := len(transport.activePosts)
		transport.mu.RUnlock()
		require.Zero(t, activePosts, "Close returned before the active POST stopped")
	})
}

func TestStreamableHTTP_JSONPOSTRequiresCorrelatedTerminalResponse(t *testing.T) {
	fixtures := []struct {
		name     string
		response string
	}{
		{
			name:     "notification",
			response: `{"jsonrpc":"2.0","method":"notifications/message","params":{}}`,
		},
		{name: "mismatched id", response: `{"jsonrpc":"2.0","id":999,"result":{}}`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, fixture.response)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.Request(context.Background(), "correlated", struct{}{})
			require.NoError(t, err)

			// Act.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = pending.Await(ctx)

			// Assert.
			var payloadErr *InvalidPayloadError
			require.ErrorAs(t, err, &payloadErr)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_ParallelPOSTStreamsResumeIndependentlyAfterRetry(t *testing.T) {
	// Arrange.
	const retryDelay = 75 * time.Millisecond
	type resumeAttempt struct {
		lastID string
		at     time.Time
	}
	postEnded := make(map[string]time.Time)
	var postMu sync.Mutex
	resumed := make(chan resumeAttempt, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			envelope := decodeRPCEnvelope(t, request)
			id := string(envelope.ID)
			postMu.Lock()
			postEnded[id] = time.Now()
			postMu.Unlock()
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(writer, "id: stream-%s\nretry: %d\n\n", id, retryDelay.Milliseconds())
		case http.MethodGet:
			lastID := request.Header.Get("Last-Event-ID")
			resumed <- resumeAttempt{lastID: lastID, at: time.Now()}
			id := strings.TrimPrefix(lastID, "stream-")
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(writer, "id: done-%s\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n", id, id)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pendingA, err := transport.Request(context.Background(), "a", struct{}{})
	require.NoError(t, err)
	pendingB, err := transport.Request(context.Background(), "b", struct{}{})
	require.NoError(t, err)

	// Act.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, errA := pendingA.Await(ctx)
	_, errB := pendingB.Await(ctx)

	// Assert.
	require.NoError(t, errA)
	require.NoError(t, errB)
	seen := make(map[string]bool)
	for range 2 {
		attempt := <-resumed
		lastID := attempt.lastID
		seen[lastID] = true
		id := strings.TrimPrefix(lastID, "stream-")
		postMu.Lock()
		startedAt := postEnded[id]
		postMu.Unlock()
		require.GreaterOrEqual(t, attempt.at.Sub(startedAt), retryDelay)
	}
	require.True(t, seen["stream-"+string(pendingA.ID())])
	require.True(t, seen["stream-"+string(pendingB.ID())])
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_Resume405CompletesPendingRequest(t *testing.T) {
	// Arrange.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: stranded\nretry: 1\n\n")
			return
		}
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "stranded", struct{}{})
	require.NoError(t, err)

	// Act.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = pending.Await(ctx)

	// Assert.
	var httpErr *HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusMethodNotAllowed, httpErr.StatusCode)
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_CancelledPendingStopsActiveResumeGET(t *testing.T) {
	// Arrange.
	getStarted := make(chan struct{})
	releaseServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: cancellable\nretry: 1\n\n")
			return
		}
		close(getStarted)
		select {
		case <-request.Context().Done():
		case <-releaseServer:
		}
	}))
	defer server.Close()
	defer close(releaseServer)
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "cancellable", struct{}{})
	require.NoError(t, err)
	<-getStarted
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	resumeStopped := make(chan struct{})
	go func() {
		transport.resumeWG.Wait()
		close(resumeStopped)
	}()
	select {
	case <-resumeStopped:
	case <-time.After(time.Second):
		t.Fatal("resume GET was not cancelled with its pending request")
	}
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_PostResumeCursorDoesNotLeakIntoActivate(t *testing.T) {
	// Arrange.
	activatedCursor := make(chan string, 1)
	var getCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			envelope := decodeRPCEnvelope(t, request)
			if envelope.Method == MethodInitialized {
				writer.WriteHeader(http.StatusAccepted)
				return
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: initialize-priming\ndata:\nretry: 1\n\n")
		case http.MethodGet:
			if getCount.Add(1) == 1 {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(
					writer,
					"id: initialize-terminal\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":%s}\n\n",
					initializeResult(ServerCapabilities{}),
				)
				return
			}
			activatedCursor <- request.Header.Get("Last-Event-ID")
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))

	// Act.
	client, err := Connect(context.Background(), transport)

	// Assert.
	require.NoError(t, err)
	select {
	case cursor := <-activatedCursor:
		require.Empty(t, cursor)
	case <-time.After(time.Second):
		t.Fatal("operation-phase GET did not start")
	}
	require.NoError(t, client.Close())
}

func TestStreamableHTTP_RejectsForgedSessionIDs(t *testing.T) {
	// Arrange.
	fixtures := []string{"contains space", "control\x1f", "delete\x7f", string([]byte{0xff})}
	for _, sessionID := range fixtures {
		// Act.
		err := validateSessionID(sessionID)

		// Assert.
		var payloadErr *InvalidPayloadError
		require.ErrorAs(t, err, &payloadErr)
	}
}

func TestStreamableHTTP_RejectsInvalidUTF8JSONAndSSE(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{
			name:        "json",
			contentType: "application/json",
			body:        []byte{'{', '"', 'j', 's', 'o', 'n', 'r', 'p', 'c', '"', ':', '"', 0xff, '"', '}'},
		},
		{name: "sse", contentType: "text/event-stream", body: []byte{'d', 'a', 't', 'a', ':', ' ', 0xff, '\n', '\n'}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", fixture.contentType)
				_, _ = writer.Write(fixture.body)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.Request(context.Background(), "utf8", struct{}{})
			require.NoError(t, err)

			// Act.
			_, err = pending.Await(context.Background())

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_OversizedGETSSEFailsClosedWithoutRetry(t *testing.T) {
	// Arrange.
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		gets.Add(1)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", strings.Repeat("x", 256))
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
		WithStreamableHTTPMaxStreamBytes(32),
	)
	require.NoError(t, transport.Start(context.Background()))

	// Act.
	transport.Activate()
	require.Eventually(t, func() bool { return transport.peer.closed.Load() }, time.Second, time.Millisecond)
	time.Sleep(2 * httpPollRetryDelay)

	// Assert.
	require.EqualValues(t, 1, gets.Load())
	_, err := transport.Request(context.Background(), "after-oversize", struct{}{})
	require.ErrorIs(t, err, ErrTransportClosed)
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_EmptySSEDataPrimesResumeCursor(t *testing.T) {
	// Arrange.
	transport := &StreamableHTTPTransport{
		maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
		peer:           newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil }),
	}

	// Act.
	err := transport.consumeSSE(
		context.Background(),
		strings.NewReader("id: primed\ndata:\n\n"),
	)

	// Assert.
	require.ErrorIs(t, err, errSSEPollingBoundary)
	require.Equal(t, "primed", transport.lastEventID)
}

func TestStreamableHTTP_SSEAcceptsStandardLineEndingsAndLeadingBOM(t *testing.T) {
	// Arrange.
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", separator), func(t *testing.T) {
			// Arrange.
			peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
			pending, _, err := peer.beginRequest("line-ending", nil)
			require.NoError(t, err)
			transport := &StreamableHTTPTransport{
				maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
				peer:           peer,
			}
			stream := "\ufeffid: cursor" + separator +
				fmt.Sprintf(`data: {"jsonrpc":"2.0","id":%s,"result":{}}`, pending.ID()) +
				separator + separator

			// Act.
			consumeErr := transport.consumeSSE(context.Background(), strings.NewReader(stream))
			result, awaitErr := pending.Await(context.Background())

			// Assert.
			require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
			require.NoError(t, awaitErr)
			require.JSONEq(t, `{}`, string(result))
			require.Equal(t, "cursor", transport.lastEventID)
		})
	}
}

func TestStreamableHTTP_SSEDiscardsIncompleteEventAtEOF(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest("incomplete", nil)
	require.NoError(t, err)
	transport := &StreamableHTTPTransport{
		maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
		peer:           peer,
	}
	stream := fmt.Sprintf(
		"id: next\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n",
		pending.ID(),
	)

	// Act.
	state, consumeErr := transport.consumeSSEState(
		context.Background(),
		strings.NewReader(stream),
		sseResumeState{lastEventID: "previous", retryDelay: httpPollRetryDelay},
		false,
	)

	// Assert.
	require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
	require.Equal(t, "previous", state.lastEventID)
	require.True(t, peer.hasPendingID(pending.ID()))
	peer.close(ErrTransportClosed)
}

func TestStreamableHTTP_SSELineUsesConfiguredStreamLimit(t *testing.T) {
	// Arrange.
	const payloadSize = rpcJSONLineScannerMaxBytes + 1024
	peer := newRPCPeer(context.Background(), nil, func(context.Context, []byte) error { return nil })
	pending, _, err := peer.beginRequest("large-line", nil)
	require.NoError(t, err)
	transport := &StreamableHTTPTransport{
		maxStreamBytes: 2 * payloadSize,
		peer:           peer,
	}
	result := strings.Repeat("x", payloadSize)
	stream := fmt.Sprintf(
		"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%q}\n\n",
		pending.ID(),
		result,
	)

	// Act.
	consumeErr := transport.consumeSSE(context.Background(), strings.NewReader(stream))
	raw, awaitErr := pending.Await(context.Background())

	// Assert.
	require.ErrorIs(t, consumeErr, errSSEPollingBoundary)
	require.NoError(t, awaitErr)
	require.Len(t, raw, payloadSize+2)
}

func TestStreamableHTTP_TerminalFailureCancelsConcurrentLifecycle(t *testing.T) {
	// Arrange.
	getStarted := make(chan struct{})
	getCancelled := make(chan struct{})
	postStarted := make(chan struct{})
	postCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", eventStreamMediaType)
			writer.WriteHeader(http.StatusOK)
			writer.(http.Flusher).Flush()
			close(getStarted)
			<-request.Context().Done()
			close(getCancelled)
			return
		}
		defer request.Body.Close()
		var message struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if message.Method == "active/request" {
			close(postStarted)
			<-request.Context().Done()
			close(postCancelled)
			return
		}
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	transport.Activate()
	<-getStarted
	active, err := transport.Request(context.Background(), "active/request", struct{}{})
	require.NoError(t, err)
	<-postStarted
	terminal, err := transport.Request(context.Background(), "terminal/request", struct{}{})
	require.NoError(t, err)

	// Act.
	_, terminalErr := terminal.Await(context.Background())
	_, activeErr := active.Await(context.Background())

	// Assert.
	var terminalHTTPError *HTTPError
	var activeHTTPError *HTTPError
	require.ErrorAs(t, terminalErr, &terminalHTTPError)
	require.ErrorAs(t, activeErr, &activeHTTPError)
	require.Equal(t, terminalHTTPError.StatusCode, activeHTTPError.StatusCode)
	select {
	case <-getCancelled:
	case <-time.After(time.Second):
		t.Fatal("terminal failure did not cancel active GET")
	}
	select {
	case <-postCancelled:
	case <-time.After(time.Second):
		t.Fatal("terminal failure did not cancel active POST")
	}
	require.Eventually(t, func() bool {
		_, requestErr := transport.Request(context.Background(), "after/terminal", struct{}{})
		return errors.Is(requestErr, ErrTransportClosed)
	}, time.Second, time.Millisecond)
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_TerminalSSEEventCancelsHeldOpenPOST(t *testing.T) {
	// Arrange.
	postCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var message struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", eventStreamMediaType)
		writer.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(
			writer,
			"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n",
			message.ID,
		)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
		close(postCancelled)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(
		server.URL,
		WithStreamableHTTPAllowPrivateIPs(true),
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "held/open", struct{}{})
	require.NoError(t, err)

	// Act.
	result, err := pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
	select {
	case <-postCancelled:
	case <-time.After(time.Second):
		t.Fatal("terminal SSE event did not cancel held-open POST")
	}
	require.Eventually(t, func() bool {
		transport.mu.RLock()
		defer transport.mu.RUnlock()
		return len(transport.activePosts) == 0
	}, time.Second, time.Millisecond)
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_RequiresExactMediaType(t *testing.T) {
	for _, contentType := range []string{"application/json-evil", "text/event-stream-bogus"} {
		t.Run(contentType, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", contentType)
				_, _ = fmt.Fprint(writer, `{}`)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.Request(context.Background(), "media", struct{}{})
			require.NoError(t, err)

			// Act.
			_, err = pending.Await(context.Background())

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_RejectsMethodChangingRedirect(t *testing.T) {
	// Arrange.
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls.Add(1)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer origin.Close()
	transport := NewStreamableHTTPTransport(origin.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "redirect", struct{}{})
	require.NoError(t, err)

	// Act.
	_, err = pending.Await(context.Background())

	// Assert.
	require.ErrorContains(t, err, "redirect must preserve HTTP method")
	require.Zero(t, targetCalls.Load())
	require.NoError(t, transport.Close())
}

func TestStreamableHTTP_Enforces202ResponseShape(t *testing.T) {
	fixtures := []struct {
		name   string
		status int
		body   string
	}{
		{name: "202 with body", status: http.StatusAccepted, body: `{}`},
		{name: "notification with 200", status: http.StatusOK, body: ""},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(fixture.status)
				_, _ = fmt.Fprint(writer, fixture.body)
			}))
			defer server.Close()
			transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
			require.NoError(t, transport.Start(context.Background()))

			// Act.
			err := transport.Notify(context.Background(), "notifications/test", struct{}{})

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStreamableHTTP_SafeClientRejectsRedirectToLoopback(t *testing.T) {
	// Arrange.
	client := defaultStreamableHTTPClient(false)
	redirect, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/private", nil)
	require.NoError(t, err)
	original, err := http.NewRequest(http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)

	// Act.
	err = client.CheckRedirect(redirect, []*http.Request{original})

	// Assert.
	require.Error(t, err)
}

func TestTransport_CloseBeforeStartIsTerminal(t *testing.T) {
	// Arrange.
	httpTransport := NewStreamableHTTPTransport("https://example.invalid/mcp")
	stdioTransport := NewStdioTransport("ignored", nil)

	// Act.
	require.NoError(t, httpTransport.Close())
	require.NoError(t, stdioTransport.Close())

	// Assert.
	require.ErrorIs(t, httpTransport.Start(context.Background()), ErrTransportClosed)
	require.ErrorIs(t, stdioTransport.Start(context.Background()), ErrTransportClosed)
	require.ErrorIs(t, httpTransport.Notify(context.Background(), "test", nil), ErrTransportClosed)
	require.ErrorIs(t, stdioTransport.Notify(context.Background(), "test", nil), ErrTransportClosed)
}

func TestStdio_ProcessExitReturnsTypedCrash(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport("/bin/sh", []string{"-c", "read line; exit 7"})
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), "crash", nil)
	require.NoError(t, err)

	// Act.
	_, err = pending.Await(context.Background())

	// Assert.
	var crashErr *TransportCrashError
	require.ErrorAs(t, err, &crashErr)
	require.NoError(t, transport.Close())
}

func TestProtocol_StrictRequiredFieldsAndTaggedUnions(t *testing.T) {
	// Arrange.
	fixtures := []struct {
		name string
		raw  string
		into any
	}{
		{name: "missing tool content", raw: `{}`, into: &CallToolResult{}},
		{name: "null resource contents", raw: `{"contents":null}`, into: &ResourcesReadResult{}},
		{name: "missing text", raw: `{"content":[{"type":"text"}]}`, into: &CallToolResult{}},
		{
			name: "cross variant field",
			raw:  `{"content":[{"type":"text","text":"x","data":"bad"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "invalid prompt role",
			raw:  `{"messages":[{"role":"system","content":{"type":"text","text":"x"}}]}`,
			into: &PromptsGetResult{},
		},
		{
			name: "resource without uri",
			raw:  `{"contents":[{"text":"x"}]}`,
			into: &ResourcesReadResult{},
		},
		{
			name: "invalid audience",
			raw:  `{"content":[{"type":"text","text":"x","annotations":{"audience":["system"]}}]}`,
			into: &CallToolResult{},
		},
		{name: "null text", raw: `{"content":[{"type":"text","text":null}]}`, into: &CallToolResult{}},
		{
			name: "null image data",
			raw:  `{"content":[{"type":"image","data":null,"mimeType":"image/png"}]}`,
			into: &CallToolResult{},
		},
		{
			name: "null resource text",
			raw:  `{"contents":[{"uri":"file:///x","text":null}]}`,
			into: &ResourcesReadResult{},
		},
		{name: "null progress", raw: `{"progressToken":"x","progress":null}`, into: &ProgressParams{}},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Act.
			err := json.Unmarshal([]byte(fixture.raw), fixture.into)

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestStructuredContent_PreservesLargeInteger(t *testing.T) {
	// Arrange.
	raw := json.RawMessage(`{"value":9007199254740993}`)

	// Act.
	canonical, _, err := canonicalJSON(raw)

	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(canonical))
	require.Contains(t, string(canonical), "9007199254740993")
}

func TestClient_NonToolCancellationUsesActiveRequestID(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		if request.Method == MethodInitialize {
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		}
		return nil, nil, false
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		for _, iterErr := range client.GetTools(ctx) {
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
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var cancellations int
	for _, notification := range transport.notifications {
		if notification.Method == MethodCancelled {
			cancellations++
		}
	}
	require.Equal(t, 1, cancellations)
}

func TestClient_ToolsInvalidationAbortsInFlightSnapshot(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{ListChanged: true}}), nil, true
		case MethodToolsList:
			transport.emit(MethodToolsListChanged, struct{}{})
			return mustJSON(t, ToolsListResult{Tools: []MCPTool{{
				Name:        "stale",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}}}), nil, true
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)

	// Act.
	var discoveryErr error
	for _, iterErr := range client.GetTools(context.Background()) {
		discoveryErr = iterErr
	}

	// Assert.
	var staleErr *StaleDiscoveryError
	require.ErrorAs(t, discoveryErr, &staleErr)
}

func TestClient_YieldAbortSendsCancellationExactlyOnce(t *testing.T) {
	// Arrange.
	transport := newFakeTransport()
	transport.requestHook = func(request capturedRequest) (json.RawMessage, error, bool) {
		switch request.Method {
		case MethodInitialize:
			return initializeResult(ServerCapabilities{Tools: &ToolsCapability{}}), nil, true
		case MethodToolsList:
			return mustJSON(
				t,
				ToolsListResult{
					Tools: []MCPTool{
						{Name: "abort", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
				},
			), nil, true
		case MethodToolsCall:
			return nil, nil, false
		default:
			return nil, errors.New("unexpected method"), true
		}
	}
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	var tool toolsy.Tool
	for discovered, iterErr := range client.GetTools(context.Background()) {
		require.NoError(t, iterErr)
		tool = discovered
	}

	result := make(chan error, 1)
	yielded := make(chan toolsy.Chunk, 1)

	// Act.
	go func() {
		result <- tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(chunk toolsy.Chunk) error {
				yielded <- chunk
				return errors.New("stop")
			},
		)
	}()
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.requests) == 3
	}, time.Second, time.Millisecond)
	transport.mu.Lock()
	toolCall := transport.requests[2]
	transport.mu.Unlock()
	var callParams ToolsCallParams
	require.NoError(t, json.Unmarshal(toolCall.Params, &callParams))
	require.NotNil(t, callParams.Meta)
	transport.emit(MethodProgress, ProgressParams{
		ProgressToken: callParams.Meta.ProgressToken,
		Progress:      1,
	})
	err = <-result

	// Assert.
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)
	progress := <-yielded
	require.Equal(t, toolsy.EventProgress, progress.Event)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var cancellations int
	for _, notification := range transport.notifications {
		if notification.Method == MethodCancelled {
			cancellations++
			var params CancelledParams
			require.NoError(t, json.Unmarshal(notification.Params, &params))
			require.JSONEq(t, string(toolCall.ID), string(params.RequestID))
		}
	}
	require.Equal(t, 1, cancellations)
}

func TestResourceResult_PreservesSingleContentMIMEAndBinary(t *testing.T) {
	// Arrange.
	text := `{"ok":true}`
	blob := "AAE="

	// Act.
	jsonChunk, err := buildResourceResultChunk(
		ResourcesReadResult{
			Contents: []ResourceContents{
				{URI: "file:///a", MIMEType: toolsy.MimeTypeJSON, Text: &text},
			},
		},
		nil,
	)
	require.NoError(t, err)
	binaryChunk, err := buildResourceResultChunk(
		ResourcesReadResult{
			Contents: []ResourceContents{
				{URI: "file:///b", MIMEType: "application/octet-stream", Blob: &blob},
			},
		},
		nil,
	)

	// Assert.
	require.NoError(t, err)
	require.Equal(t, toolsy.MimeTypeJSON, jsonChunk.MimeType)
	require.JSONEq(t, text, string(jsonChunk.Data))
	require.Equal(t, []byte{0, 1}, binaryChunk.Data)
	require.Equal(t, toolsy.DeliveryClassBinary, binaryChunk.Envelope.DeliveryClass)
}
