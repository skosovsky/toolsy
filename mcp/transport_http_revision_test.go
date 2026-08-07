package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingStdioWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	frames  [][]byte
}

func newBlockingStdioWriter() *blockingStdioWriter {
	return &blockingStdioWriter{started: make(chan struct{}), release: make(chan struct{})}
}

func (w *blockingStdioWriter) Write(data []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	w.mu.Lock()
	w.frames = append(w.frames, bytes.Clone(data))
	w.mu.Unlock()
	return len(data), nil
}

func (w *blockingStdioWriter) Close() error { return nil }

func (w *blockingStdioWriter) snapshot() [][]byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	frames := make([][]byte, len(w.frames))
	for index := range w.frames {
		frames[index] = bytes.Clone(w.frames[index])
	}
	return frames
}

func newAdversarialStdioTransport(
	t *testing.T,
	opts ...StdioTransportOption,
) (*StdioTransport, *blockingStdioWriter, context.CancelFunc) {
	t.Helper()
	lifetime, cancel := context.WithCancel(context.Background())
	writer := newBlockingStdioWriter()
	transport := NewStdioTransport("unused", nil, opts...)
	transport.started = true
	transport.lifetimeCtx = lifetime
	transport.cancel = cancel
	transport.stdin = writer
	transport.peer = newRPCPeer(lifetime, slog.Default(), transport.send)
	go transport.writeLoop(lifetime, writer)
	return transport, writer, func() {
		cancel()
		<-transport.writerDone
	}
}

func TestPreparedRequestRegistersSubscriptionBeforeImmediateACK(t *testing.T) {
	// Arrange.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var envelope Request
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch envelope.Method {
		case MethodServerDiscover:
			writer.Header().Set("Content-Type", "application/json")
			discovery := `{"jsonrpc":"2.0","id":%s,"result":{"resultType":"complete",` +
				`"supportedVersions":["%s"],"capabilities":{"tools":{"listChanged":true}},` +
				`"ttlMs":0,"cacheScope":"private"}}`
			_, _ = fmt.Fprintf(writer, discovery, envelope.ID, ProtocolVersion)
		case MethodSubscriptionsListen:
			writer.Header().Set("Content-Type", eventStreamMediaType)
			ackFormat := `{"jsonrpc":"2.0","method":"%s","params":{` +
				`"notifications":{"toolsListChanged":true},"_meta":{"%s":%s}}}`
			terminalFormat := `{"jsonrpc":"2.0","id":%s,"result":{` +
				`"resultType":"complete","_meta":{"%s":%s}}}`
			_, _ = fmt.Fprintf(writer, "data: "+ackFormat+"\n\ndata: "+terminalFormat+"\n\n",
				MethodSubscriptionsAcknowledged, metaSubscriptionID, envelope.ID,
				envelope.ID, metaSubscriptionID, envelope.ID,
			)
		default:
			http.Error(writer, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	defer client.Close()

	// Act.
	subscription, err := client.Listen(context.Background(), SubscriptionFilter{ToolsListChanged: true})
	require.NoError(t, err)
	select {
	case <-subscription.Done():
	case <-time.After(time.Second):
		t.Fatal("subscription did not complete")
	}

	// Assert.
	require.NoError(t, subscription.Err())
}

func TestPreparedRequestDeliveryBarrierIsExactlyOnce(t *testing.T) {
	// Arrange.
	var mu sync.Mutex
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		posts++
		mu.Unlock()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var envelope Request
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		discovery := `{"jsonrpc":"2.0","id":%s,"result":{"resultType":"complete",` +
			`"supportedVersions":["%s"],"capabilities":{},"ttlMs":0,"cacheScope":"private"}}`
		_, _ = fmt.Fprintf(writer, discovery, envelope.ID, ProtocolVersion)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	defer transport.Close()
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	mu.Lock()
	require.Zero(t, posts)
	mu.Unlock()

	// Act.
	require.NoError(t, pending.Deliver())
	secondDeliverErr := pending.Deliver()
	_, awaitErr := pending.Await(context.Background())

	// Assert.
	require.ErrorIs(t, secondDeliverErr, errRequestAlreadyDelivered)
	require.NoError(t, awaitErr)
	mu.Lock()
	require.Equal(t, 1, posts)
	mu.Unlock()
	abortCause := errors.New("registration failed")
	aborted, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, aborted.Abort(abortCause))
	_, abortedErr := aborted.Await(context.Background())
	require.ErrorIs(t, abortedErr, abortCause)
	require.False(t, aborted.(DeliveryPendingRequest).WasSent())
	require.ErrorIs(t, aborted.Deliver(), errRequestAlreadyDelivered)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, posts)
}

func TestStreamableHTTPPreparedRequestSnapshotsToolRoutingHeaders(t *testing.T) {
	// Arrange.
	var captured http.Header
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured = request.Header.Clone()
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var envelope Request
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer,
			`{"jsonrpc":"2.0","id":%s,"result":{"resultType":"complete","content":[]}}`,
			envelope.ID,
		)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	defer transport.Close()
	_, err := transport.PrepareRequest(context.Background(), MethodToolsCall, ToolsCallParams{
		Name: "lookup", Arguments: json.RawMessage(`{"tenant":"north"}`), Meta: requestMetaForContract(),
	})
	require.ErrorContains(t, err, "no current tools/list header descriptor")
	require.NoError(t, transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"lookup": {{Header: "Tenant", Path: []string{"tenant"}, Type: schemaTypeString}},
	}))
	pending, err := transport.PrepareRequest(context.Background(), MethodToolsCall, ToolsCallParams{
		Name: "lookup", Arguments: json.RawMessage(`{"tenant":"north"}`), Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, transport.ReplaceToolHeaderBindings(nil))

	// Act.
	require.NoError(t, pending.Deliver())
	_, awaitErr := pending.Await(context.Background())

	// Assert.
	require.NoError(t, awaitErr)
	require.Equal(t, "north", captured.Get("Mcp-Param-Tenant"))
}

func TestStreamableHTTPCancellationClosesOnlyOriginatingPOST(t *testing.T) {
	// Arrange.
	var mu sync.Mutex
	methods := make([]string, 0, 2)
	requestStarted := make(chan struct{})
	requestCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var envelope Request
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		mu.Lock()
		methods = append(methods, envelope.Method)
		mu.Unlock()
		if envelope.Method == MethodServerDiscover {
			writer.Header().Set("Content-Type", "application/json")
			discovery := `{"jsonrpc":"2.0","id":%s,"result":{"resultType":"complete",` +
				`"supportedVersions":["%s"],"capabilities":{},"ttlMs":0,"cacheScope":"private"}}`
			_, _ = fmt.Fprintf(writer, discovery, envelope.ID, ProtocolVersion)
			return
		}
		close(requestStarted)
		<-request.Context().Done()
		close(requestCancelled)
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	client, err := Connect(context.Background(), transport)
	require.NoError(t, err)
	defer client.Close()
	requestCtx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, requestErr := client.requestAndAwait(requestCtx, MethodToolsList, ToolsListParams{})
		result <- requestErr
	}()
	<-requestStarted

	// Act.
	cancel()
	requestErr := <-result
	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP response stream was not closed on cancellation")
	}

	// Assert.
	require.ErrorIs(t, requestErr, context.Canceled)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{MethodServerDiscover, MethodToolsList}, methods)
}

func TestStdioPostDeliveryCancellationIsEmittedExactlyOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	// Arrange.
	outputPath := filepath.Join(t.TempDir(), "frames.jsonl")
	transport := NewStdioTransport("/bin/sh", []string{
		"-c", `while IFS= read -r line; do printf '%s\n' "$line" >> "$1"; done`, "mcp-fixture", outputPath,
	})
	require.NoError(t, transport.Start(context.Background()))
	defer transport.Close()
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	delivery := pending.(DeliveryPendingRequest)
	select {
	case <-delivery.DeliveryDone():
	case <-time.After(time.Second):
		t.Fatal("stdio request was not delivered")
	}
	require.True(t, delivery.WasSent())

	// Act.
	require.True(t, pending.(CancellablePendingRequest).CancelPending())
	notification := pending.(CancellationNotificationPending)
	require.True(t, notification.ClaimCancellationNotification())
	require.NoError(t, transport.Notify(context.Background(), MethodCancelled, CancelledParams{
		RequestID: pending.ID(), Reason: "test cancellation",
	}))
	require.False(t, notification.ClaimCancellationNotification())

	// Assert.
	require.Eventually(t, func() bool {
		raw, readErr := os.ReadFile(outputPath)
		return readErr == nil && len(strings.Split(strings.TrimSpace(string(raw)), "\n")) == 2
	}, time.Second, 10*time.Millisecond)
	raw, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var cancellation Notification
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &cancellation))
	require.Equal(t, MethodCancelled, cancellation.Method)
}

func TestStdioBlockingWriteCancellationPreservesDeliveredFrameAndTransport(t *testing.T) {
	// Arrange.
	transport, writer, stop := newAdversarialStdioTransport(t)
	defer stop()
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	<-writer.started

	// Act. Cancellation races after Write started but before it returns.
	require.True(t, pending.(CancellablePendingRequest).CancelPending())
	close(writer.release)
	<-pending.(DeliveryPendingRequest).DeliveryDone()

	var claims atomic.Int32
	var claimers sync.WaitGroup
	notifyResult := make(chan error, 1)
	for range 8 {
		claimers.Go(func() {
			if pending.(CancellationNotificationPending).ClaimCancellationNotification() {
				claims.Add(1)
				notifyResult <- transport.Notify(t.Context(), MethodCancelled, CancelledParams{
					RequestID: pending.ID(), Reason: "blocked write completed",
				})
			}
		})
	}
	claimers.Wait()
	require.NoError(t, <-notifyResult)
	require.NoError(t, transport.Notify(t.Context(), "notifications/vendor.example/alive", map[string]any{}))

	// Assert. The request was delivered, cancellation emitted once, and an
	// unrelated write still succeeds on the same transport.
	require.True(t, pending.(DeliveryPendingRequest).WasSent())
	require.EqualValues(t, 1, claims.Load())
	require.Eventually(t, func() bool { return len(writer.snapshot()) == 3 }, time.Second, time.Millisecond)
	methods := make([]string, 0, 3)
	for _, frame := range writer.snapshot() {
		var envelope rpcEnvelope
		require.NoError(t, json.Unmarshal(frame, &envelope))
		methods = append(methods, envelope.Method)
	}
	require.Equal(t, []string{MethodServerDiscover, MethodCancelled, "notifications/vendor.example/alive"}, methods)
}

func TestStdioCancellationRetractsQueuedRequestWithoutNotification(t *testing.T) {
	// Arrange. Occupy the writer so the prepared request remains retractably queued.
	transport, writer, stop := newAdversarialStdioTransport(t)
	defer stop()
	blockerDone := make(chan error, 1)
	go func() { blockerDone <- transport.send(t.Context(), []byte(`{"jsonrpc":"2.0","method":"blocker"}`)) }()
	<-writer.started
	pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	require.Eventually(t, func() bool { return len(transport.writeQueue) == 1 }, time.Second, time.Millisecond)

	// Act.
	require.True(t, pending.(CancellablePendingRequest).CancelPending())
	<-pending.(DeliveryPendingRequest).DeliveryDone()
	if pending.(DeliveryPendingRequest).WasSent() &&
		pending.(CancellationNotificationPending).ClaimCancellationNotification() {
		require.NoError(t, transport.Notify(t.Context(), MethodCancelled, CancelledParams{RequestID: pending.ID()}))
	}
	close(writer.release)
	require.NoError(t, <-blockerDone)
	require.NoError(t, transport.Notify(t.Context(), "notifications/vendor.example/alive", map[string]any{}))

	// Assert. Neither the request nor notifications/cancelled reached the wire.
	require.False(t, pending.(DeliveryPendingRequest).WasSent())
	require.Eventually(t, func() bool { return len(writer.snapshot()) == 2 }, time.Second, time.Millisecond)
	frames := writer.snapshot()
	require.Contains(t, string(frames[0]), `"method":"blocker"`)
	require.Contains(t, string(frames[1]), `"method":"notifications/vendor.example/alive"`)
	for _, frame := range frames {
		require.NotContains(t, string(frame), MethodCancelled)
		require.NotContains(t, string(frame), MethodServerDiscover)
	}
}

func TestStdioBlockingWriteCancellationRaceCount(t *testing.T) {
	for iteration := range 128 {
		transport, writer, stop := newAdversarialStdioTransport(t)
		pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
			Meta: requestMetaForContract(),
		})
		require.NoError(t, err, iteration)
		require.NoError(t, pending.Deliver(), iteration)
		<-writer.started
		require.True(t, pending.(CancellablePendingRequest).CancelPending(), iteration)
		close(writer.release)
		<-pending.(DeliveryPendingRequest).DeliveryDone()
		require.True(t, pending.(DeliveryPendingRequest).WasSent(), iteration)
		transport.mu.Lock()
		closed := transport.closed
		transport.mu.Unlock()
		require.False(t, closed, iteration)
		stop()
	}
}

func TestStdioOutgoingFrameLimitRejectsRequestAndNotificationBeforeEnqueue(t *testing.T) {
	// Arrange.
	transport, writer, stop := newAdversarialStdioTransport(t, WithStdioMaxStreamBytes(512))
	defer stop()

	// Act.
	pending, requestErr := transport.PrepareRequest(t.Context(), MethodToolsCall, ToolsCallParams{
		Name:      "oversized",
		Arguments: json.RawMessage(`{"payload":"` + strings.Repeat("x", 1024) + `"}`),
		Meta:      requestMetaForContract(),
	})
	notifyErr := transport.Notify(t.Context(), "notifications/vendor.example/oversized", map[string]any{
		"payload": strings.Repeat("x", 1024),
	})

	// Assert.
	require.Nil(t, pending)
	for _, err := range []error{requestErr, notifyErr} {
		var invalid *InvalidPayloadError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, "stdio outgoing frame", invalid.Subject)
	}
	require.Empty(t, transport.writeQueue)
	require.Empty(t, writer.snapshot())
	transport.mu.Lock()
	require.Zero(t, transport.queuedBytes)
	transport.mu.Unlock()
}

func TestStdioWriteQueueByteBudgetReleasesAcrossSixtyFourCancellationRaces(t *testing.T) {
	// Arrange.
	const byteBudget = 4096
	transport, writer, stop := newAdversarialStdioTransport(t, WithStdioMaxStreamBytes(byteBudget))
	defer stop()
	blockerDone := make(chan error, 1)
	go func() { blockerDone <- transport.send(t.Context(), []byte(`{"jsonrpc":"2.0","method":"blocker"}`)) }()
	<-writer.started
	type sendResult struct{ err error }
	results := make(chan sendResult, 64)
	cancels := make([]context.CancelFunc, 0, 64)
	for index := range 64 {
		ctx, cancel := context.WithCancel(t.Context())
		cancels = append(cancels, cancel)
		go func() {
			err := transport.Notify(ctx, "notifications/vendor.example/queued", map[string]any{
				"index": index, "payload": strings.Repeat("q", 192),
			})
			results <- sendResult{err: err}
		}()
	}
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		queued := transport.queuedBytes
		transport.mu.Unlock()
		return queued > 0 && len(results) > 0
	}, time.Second, time.Millisecond)

	// Act. A legal single frame is rejected because retained queued bytes own
	// the remaining budget. Cancelling all producers must not double-release.
	aggregateErr := transport.send(t.Context(), bytes.Repeat([]byte("x"), byteBudget-1))
	for _, cancel := range cancels {
		cancel()
	}
	queueErrors := 0
	for range 64 {
		result := <-results
		var invalid *InvalidPayloadError
		if errors.As(result.err, &invalid) && invalid.Subject == "stdio outgoing queue" {
			queueErrors++
		}
	}
	close(writer.release)
	require.NoError(t, <-blockerDone)

	// Assert.
	var aggregateInvalid *InvalidPayloadError
	require.ErrorAs(t, aggregateErr, &aggregateInvalid)
	require.Equal(t, "stdio outgoing queue", aggregateInvalid.Subject)
	require.Positive(t, queueErrors)
	require.Eventually(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return transport.queuedBytes == 0 && len(transport.writeQueue) == 0
	}, time.Second, time.Millisecond)
	require.Len(t, writer.snapshot(), 1)
}

func TestRequestScopedSSEFailsOnEOFAndByteLimit(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transport := NewStreamableHTTPTransport("https://example.com/mcp", WithStreamableHTTPMaxStreamBytes(128))
	transport.peer = newRPCPeer(ctx, nil, func(context.Context, []byte) error { return nil })
	transport.peer.setNotificationHandler(MethodProgress, func(json.RawMessage) {})
	provenance := &sseRequestProvenance{
		requestID:     json.RawMessage(`1`),
		method:        MethodToolsCall,
		progressToken: json.RawMessage(`"p"`),
		hasProgress:   true,
	}
	unterminated := `data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"p","progress":1}}` + "\n\n"

	// Act.
	eofErr := transport.consumeRequestSSE(ctx, strings.NewReader(unterminated), provenance)
	limitErr := transport.consumeRequestSSE(ctx, strings.NewReader(strings.Repeat("x", 256)), provenance)

	// Assert.
	require.ErrorContains(t, eofErr, "before terminal response")
	require.Error(t, limitErr)
}

func TestHTTPRequestScopedLogUsesStreamRequestIDWithoutWireToken(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transport := NewStreamableHTTPTransport("https://example.com/mcp")
	transport.peer = newRPCPeer(ctx, nil, func(context.Context, []byte) error { return nil })
	pending, _, err := transport.peer.beginRequest(MethodToolsList, ToolsListParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	provenance := &sseRequestProvenance{
		requestID: pending.ID(),
		method:    MethodToolsList,
		logging:   true,
	}
	var contextualCount atomic.Int32
	var globalCount atomic.Int32
	var receivedID json.RawMessage
	transport.OnNotification(MethodLogMessage, func(json.RawMessage) { globalCount.Add(1) })
	transport.OnRequestNotification(MethodLogMessage, func(requestID, params json.RawMessage) {
		contextualCount.Add(1)
		receivedID = bytes.Clone(requestID)
		require.NotContains(t, string(params), progressTokenField)
	})
	stream := fmt.Sprintf(
		"data: {\"jsonrpc\":\"2.0\",\"method\":%q,\"params\":{\"level\":\"info\",\"data\":\"ok\"}}\n\n"+
			"data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n\n"+
			"data: {\"jsonrpc\":\"2.0\",\"method\":%q,\"params\":{\"level\":\"info\",\"data\":\"late\"}}\n\n",
		MethodLogMessage,
		pending.ID(),
		MethodLogMessage,
	)

	// Act.
	consumeErr := transport.consumeRequestSSE(ctx, strings.NewReader(stream), provenance)

	// Assert. The valid log is delivered through transport provenance, never
	// duplicated through the global handler, and the late frame is rejected.
	require.ErrorContains(t, consumeErr, "message after terminal response")
	require.EqualValues(t, 1, contextualCount.Load())
	require.Zero(t, globalCount.Load())
	require.JSONEq(t, string(pending.ID()), string(receivedID))
}

func TestTransportBoundaryRejectsEnvelopeBypassForHTTPAndStdio(t *testing.T) {
	// Arrange.
	transports := map[string]Transport{
		"http":  NewStreamableHTTPTransport("https://example.com/mcp"),
		"stdio": NewStdioTransport("unused", nil),
	}
	tests := map[string]json.RawMessage{
		"missing client info": json.RawMessage(
			`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
				`"io.modelcontextprotocol/clientCapabilities":{}}}`,
		),
		"recursive duplicate": json.RawMessage(
			`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
				`"io.modelcontextprotocol/clientCapabilities":{},` +
				`"io.modelcontextprotocol/clientInfo":{"name":"a","name":"b","version":"1"}}}`,
		),
		"unknown reserved metadata": json.RawMessage(
			`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
				`"io.modelcontextprotocol/clientCapabilities":{},` +
				`"io.modelcontextprotocol/clientInfo":{"name":"a","version":"1"},` +
				`"dev.mcp/forged":true}}`,
		),
		"legacy capability bypass": json.RawMessage(
			`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
				`"io.modelcontextprotocol/clientCapabilities":{"roots":{}},` +
				`"io.modelcontextprotocol/clientInfo":{"name":"a","version":"1"}}}`,
		),
	}

	for transportName, transport := range transports {
		for caseName, params := range tests {
			t.Run(transportName+"/"+caseName, func(t *testing.T) {
				// Act.
				pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, params)

				// Assert.
				require.Error(t, err)
				require.Nil(t, pending)
				var invalid *InvalidPayloadError
				require.ErrorAs(t, err, &invalid)
			})
		}
	}
}

func TestTransportBoundaryRejectsLegacyMethodsAndNotifications(t *testing.T) {
	// Arrange.
	stdio := NewStdioTransport("unused", nil)
	params := DiscoverParams{Meta: requestMetaForContract()}
	legacyMethods := []string{
		"initialize", legacyNotificationInitialized, "roots/list", "logging/setLevel",
		"resources/subscribe", "resources/unsubscribe", "ping",
	}

	for _, method := range legacyMethods {
		t.Run("request/"+method, func(t *testing.T) {
			// Act.
			pending, err := stdio.PrepareRequest(t.Context(), method, params)

			// Assert.
			require.Error(t, err)
			require.Nil(t, pending)
		})
	}
	for _, method := range []string{legacyNotificationInitialized, "notifications/roots/list_changed"} {
		t.Run("notification/"+method, func(t *testing.T) {
			// Act.
			err := stdio.Notify(t.Context(), method, NotificationParams{})

			// Assert.
			require.Error(t, err)
		})
	}
}

func TestPeerFailsClosedOnLegacyNotificationAndIgnoresUnknownExtensionNotification(t *testing.T) {
	// Arrange.
	peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })

	// Act.
	legacyErr := peer.dispatch([]byte(
		`{"jsonrpc":"2.0","method":"notifications/roots/list_changed","params":{}}`,
	))
	extensionErr := peer.dispatch([]byte(
		`{"jsonrpc":"2.0","method":"notifications/com.example.changed","params":{}}`,
	))

	// Assert.
	require.ErrorContains(t, legacyErr, "legacy method")
	require.NoError(t, extensionErr)
}

func TestPendingDeliveryAndCancellationClaimsRemainAtomic(t *testing.T) {
	for range 512 {
		// Arrange.
		peer := newRPCPeer(t.Context(), nil, func(context.Context, []byte) error { return nil })
		pending, _, err := peer.beginRequest(MethodServerDiscover, DiscoverParams{
			Meta: requestMetaForContract(),
		})
		require.NoError(t, err)
		start := make(chan struct{})
		var race sync.WaitGroup
		race.Add(2)

		// Act.
		go func() {
			defer race.Done()
			<-start
			pending.markSent()
		}()
		go func() {
			defer race.Done()
			<-start
			pending.CancelPending()
		}()
		close(start)
		race.Wait()
		claims := 0
		var claimRace sync.WaitGroup
		claimRace.Add(8)
		var claimMu sync.Mutex
		for range 8 {
			go func() {
				defer claimRace.Done()
				if pending.ClaimCancellationNotification() {
					claimMu.Lock()
					claims++
					claimMu.Unlock()
				}
			}()
		}
		claimRace.Wait()

		// Assert.
		require.True(t, pending.WasSent())
		select {
		case <-pending.DeliveryDone():
		default:
			t.Fatal("delivery boundary did not settle")
		}
		require.Equal(t, 1, claims)
		peer.close(ErrTransportClosed)
	}
}

func TestHTTPToolHeaderCompilerAllowsOnlyTopLevelProperties(t *testing.T) {
	// Arrange.
	tests := map[string]string{
		"nested": `{"type":"object","properties":{"outer":{"type":"object","properties":{"inner":{"type":"string","x-mcp-header":"Inner"}}}}}`,
		"ref":    `{"type":"object","properties":{"value":{"$ref":"#/$defs/value"}},"$defs":{"value":{"type":"string","x-mcp-header":"Ref"}}}`,
		"array":  `{"type":"object","properties":{"values":{"type":"array","items":{"type":"string","x-mcp-header":"Item"}}}}`,
		"allOf":  `{"type":"object","allOf":[{"properties":{"value":{"type":"string","x-mcp-header":"Composed"}}}]}`,
	}

	for name, schema := range tests {
		t.Run(name, func(t *testing.T) {
			// Act.
			bindings, err := compileHTTPToolHeaderBindings(json.RawMessage(schema))

			// Assert.
			require.Error(t, err)
			require.Empty(t, bindings)
		})
	}

	// Act.
	bindings, err := compileHTTPToolHeaderBindings(json.RawMessage(
		`{"type":"object","properties":{"tenant":{"type":"string","x-mcp-header":"Tenant"}}}`,
	))

	// Assert.
	require.NoError(t, err)
	require.Equal(t, []HTTPToolHeaderBinding{{Header: "Tenant", Path: []string{"tenant"}, Type: "string"}}, bindings)
}

func TestStreamableHTTPMissingOrNullArgumentsOmitToolParameterHeaders(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport("https://example.com/mcp")
	require.NoError(t, transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"lookup": {{Header: "Tenant", Path: []string{"tenant"}, Type: schemaTypeString}},
	}))

	for name, arguments := range map[string]json.RawMessage{"missing": nil, "null": json.RawMessage(jsonNull)} {
		t.Run(name, func(t *testing.T) {
			// Act.
			headers := make(map[string]string)
			err := transport.addToolParameterHeaders(headers, "lookup", arguments)

			// Assert.
			require.NoError(t, err)
			require.NotContains(t, headers, "Mcp-Param-Tenant")
		})
	}
}

func TestHTTPToolHeaderBindingsReplaceAtomicallyAndRemoveStaleAuthority(t *testing.T) {
	// Arrange.
	transport := NewStreamableHTTPTransport("https://example.com/mcp")
	require.NoError(t, transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"old": {{Header: "Old", Path: []string{"old"}, Type: schemaTypeString}},
	}))

	// Act. One invalid descriptor must reject the entire candidate snapshot.
	err := transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"new": {{Header: "New", Path: []string{"new"}, Type: schemaTypeString}},
		"bad": {{Header: "Bad", Path: []string{"bad"}, Type: "object"}},
	})

	// Assert rollback, then successful replacement removes stale authority.
	require.ErrorContains(t, err, "unsupported type")
	transport.mu.RLock()
	require.Contains(t, transport.toolHeaders, "old")
	require.NotContains(t, transport.toolHeaders, "new")
	transport.mu.RUnlock()
	require.NoError(t, transport.ReplaceToolHeaderBindings(map[string][]HTTPToolHeaderBinding{
		"new": {{Header: "New", Path: []string{"new"}, Type: schemaTypeString}},
	}))
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	require.NotContains(t, transport.toolHeaders, "old")
	require.Contains(t, transport.toolHeaders, "new")
}

func TestMCPHeaderValueEncodingRejectsTabControlsAndSentinelAmbiguity(t *testing.T) {
	// Arrange.
	tests := map[string]string{
		"tab":      "a\tb",
		"newline":  "a\nb",
		"sentinel": "=?base64?literal?=",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			// Act.
			encoded := encodeMCPHeaderValue(value)

			// Assert.
			require.Contains(t, encoded, "=?base64?")
			require.NotEqual(t, value, encoded)
		})
	}
	require.Equal(t, "alpha beta", encodeMCPHeaderValue("alpha beta"))
}

func TestSSEProvenanceRejectsCrossRequestAndPreAcknowledgementNotifications(t *testing.T) {
	// Arrange.
	requestProvenance := &sseRequestProvenance{
		requestID:     json.RawMessage(`1`),
		method:        MethodToolsCall,
		progressToken: json.RawMessage(`"progress-1"`),
		hasProgress:   true,
	}
	subscription := &sseRequestProvenance{
		requestID: json.RawMessage(`"listen-1"`),
		method:    MethodSubscriptionsListen,
	}

	// Act.
	_, _, progressErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"other","progress":1}}`,
	), requestProvenance)
	_, _, logErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"x"}}`,
	), requestProvenance)
	_, _, beforeAckErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","method":"notifications/tools/list_changed","params":{"_meta":{"io.modelcontextprotocol/subscriptionId":"listen-1"}}}`,
	), subscription)
	_, _, wrongAckErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","method":"notifications/subscriptions/acknowledged","params":{"_meta":{"io.modelcontextprotocol/subscriptionId":"other"},"notifications":{}}}`,
	), subscription)
	_, acknowledged, ackErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","method":"notifications/subscriptions/acknowledged","params":{"_meta":{"io.modelcontextprotocol/subscriptionId":"listen-1"},"notifications":{}}}`,
	), subscription)

	// Assert.
	require.ErrorContains(t, progressErr, "progress token mismatch")
	require.ErrorContains(t, logErr, "unsolicited logging")
	require.ErrorContains(t, beforeAckErr, "acknowledgement must be first")
	require.ErrorContains(t, wrongAckErr, "subscription id")
	require.NoError(t, ackErr)
	require.True(t, acknowledged)
}

func TestStreamableHTTPCorrelatedBadRequestPreservesRPCError(t *testing.T) {
	// Arrange.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":` + string(envelope.ID) +
			`,"error":{"code":-32020,"message":"Header mismatch"}}`))
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(context.Background()))
	defer transport.Close()

	// Act.
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	_, awaitErr := pending.Await(context.Background())

	// Assert.
	var headerErr *HeaderMismatchError
	require.ErrorAs(t, awaitErr, &headerErr)
}

func TestStreamableHTTPNon2xxAcceptsOnlyCorrelatedRPCErrorMatrix(t *testing.T) {
	statuses := []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError}
	responses := map[string]struct {
		body       func(json.RawMessage) string
		rpcFailure bool
	}{
		"correlated generic error": {
			body: func(id json.RawMessage) string {
				return `{"jsonrpc":"2.0","id":` + string(id) +
					`,"error":{"code":-32603,"message":"generic failure"}}`
			},
			rpcFailure: true,
		},
		"correlated success": {
			body: func(id json.RawMessage) string {
				return `{"jsonrpc":"2.0","id":` + string(id) + `,"result":{}}`
			},
		},
		"malformed error": {
			body: func(id json.RawMessage) string {
				return `{"jsonrpc":"2.0","id":` + string(id) +
					`,"error":{"code":"bad","message":"broken"}}`
			},
		},
		"uncorrelated error": {
			body: func(json.RawMessage) string {
				return `{"jsonrpc":"2.0","id":"other",` +
					`"error":{"code":-32020,"message":"wrong request"}}`
			},
		},
	}

	for _, status := range statuses {
		for name, response := range responses {
			t.Run(fmt.Sprintf("%d/%s", status, name), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					body, err := io.ReadAll(request.Body)
					if err != nil {
						t.Errorf("read request body: %v", err)
						return
					}
					var envelope Request
					if err := json.Unmarshal(body, &envelope); err != nil {
						t.Errorf("decode request: %v", err)
						return
					}
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(response.body(envelope.ID)))
				}))
				defer server.Close()
				transport := NewStreamableHTTPTransport(
					server.URL,
					WithStreamableHTTPAllowPrivateIPs(true),
				)
				require.NoError(t, transport.Start(t.Context()))
				defer transport.Close()
				pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
					Meta: requestMetaForContract(),
				})
				require.NoError(t, err)

				require.NoError(t, pending.Deliver())
				_, awaitErr := pending.Await(t.Context())

				if response.rpcFailure {
					var rpcErr *RPCError
					require.ErrorAs(t, awaitErr, &rpcErr)
					return
				}
				var httpErr *HTTPError
				require.ErrorAs(t, awaitErr, &httpErr)
				require.Equal(t, status, httpErr.StatusCode)
			})
		}
	}
}

func TestHTTPStatusRPCSemanticsMatrix(t *testing.T) {
	statuses := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}
	responses := map[string]func(json.RawMessage) string{
		"reserved error": func(id json.RawMessage) string {
			return `{"jsonrpc":"2.0","id":` + string(id) +
				`,"error":{"code":-32020,"message":"reserved"}}`
		},
		"generic error": func(id json.RawMessage) string {
			return `{"jsonrpc":"2.0","id":` + string(id) +
				`,"error":{"code":-32603,"message":"generic"}}`
		},
		"result": func(id json.RawMessage) string {
			return `{"jsonrpc":"2.0","id":` + string(id) + `,"result":{"ok":true}}`
		},
		"malformed": func(id json.RawMessage) string {
			return `{"jsonrpc":"2.0","id":` + string(id) +
				`,"error":{"code":"broken","message":"bad"}}`
		},
	}

	for _, status := range statuses {
		for kind, responseBody := range responses {
			t.Run(fmt.Sprintf("%d/%s", status, kind), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					body, err := io.ReadAll(request.Body)
					if err != nil {
						t.Errorf("read request body: %v", err)
						return
					}
					var requestEnvelope Request
					if err := json.Unmarshal(body, &requestEnvelope); err != nil {
						t.Errorf("decode request body: %v", err)
						return
					}
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(status)
					_, _ = writer.Write([]byte(responseBody(requestEnvelope.ID)))
				}))
				defer server.Close()
				transport := NewStreamableHTTPTransport(
					server.URL,
					WithStreamableHTTPAllowPrivateIPs(true),
				)
				require.NoError(t, transport.Start(t.Context()))
				defer transport.Close()
				pending, err := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
					Meta: requestMetaForContract(),
				})
				require.NoError(t, err)
				require.NoError(t, pending.Deliver())

				result, awaitErr := pending.Await(t.Context())

				switch {
				case status == http.StatusOK && kind == "result":
					require.NoError(t, awaitErr)
					require.JSONEq(t, `{"ok":true}`, string(result))
				case status == http.StatusOK && kind == "generic error":
					var rpcErr *RPCError
					require.ErrorAs(t, awaitErr, &rpcErr)
				case status == http.StatusOK:
					var invalid *InvalidPayloadError
					require.ErrorAs(t, awaitErr, &invalid)
				case status == http.StatusBadRequest && kind == "reserved error":
					var headerErr *HeaderMismatchError
					require.ErrorAs(t, awaitErr, &headerErr)
				case kind == "generic error":
					var rpcErr *RPCError
					require.ErrorAs(t, awaitErr, &rpcErr)
				default:
					var httpErr *HTTPError
					require.ErrorAs(t, awaitErr, &httpErr)
					require.Equal(t, status, httpErr.StatusCode)
				}
			})
		}
	}
}

func TestReservedHTTP400ErrorsAreRejectedOnDirectAndSSEResponses(t *testing.T) {
	reservedCodes := []JSONNumber{
		JSONRPCHeaderMismatch,
		JSONRPCMissingRequiredClientCapability,
		JSONRPCUnsupportedProtocolVersion,
	}
	for _, code := range reservedCodes {
		payload := fmt.Appendf(nil,
			`{"jsonrpc":"2.0","id":1,"error":{"code":%s,"message":"reserved"}}`,
			code,
		)
		directErr := rejectReservedRPCErrorOutsideHTTP400(payload)
		_, _, sseErr := validateSSEProvenance(payload, &sseRequestProvenance{
			requestID: json.RawMessage(`1`),
			method:    MethodToolsList,
		})
		require.ErrorContains(t, directErr, "requires HTTP 400")
		require.ErrorContains(t, sseErr, "requires HTTP 400")
	}
}

func TestSubscriptionSSEAllowsTypedTerminalErrorBeforeAcknowledgement(t *testing.T) {
	// Arrange.
	preACK := &sseRequestProvenance{
		requestID: json.RawMessage(`1`),
		method:    MethodSubscriptionsListen,
	}
	terminalError, _, provenanceErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"rejected"}}`,
	), preACK)
	_, _, successErr := validateSSEProvenance([]byte(
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
	), preACK)
	require.NoError(t, provenanceErr)
	require.True(t, terminalError)
	require.ErrorContains(t, successErr, "before acknowledgement")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var envelope Request
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch envelope.Method {
		case MethodSubscriptionsListen:
			writer.Header().Set("Content-Type", eventStreamMediaType)
			_, _ = fmt.Fprintf(writer,
				"data: {\"jsonrpc\":\"2.0\",\"id\":%s,"+
					"\"error\":{\"code\":-32603,\"message\":\"subscription rejected\"}}\n\n",
				envelope.ID,
			)
		default:
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%s,"result":{"alive":true}}`, envelope.ID)
		}
	}))
	defer server.Close()
	transport := NewStreamableHTTPTransport(server.URL, WithStreamableHTTPAllowPrivateIPs(true))
	require.NoError(t, transport.Start(t.Context()))
	defer transport.Close()
	listen, err := transport.PrepareRequest(t.Context(), MethodSubscriptionsListen, SubscriptionsListenParams{
		Notifications: SubscriptionFilter{ToolsListChanged: true},
		Meta:          requestMetaForContract(),
	})
	require.NoError(t, err)

	// Act.
	require.NoError(t, listen.Deliver())
	_, listenErr := listen.Await(t.Context())
	probe, probeErr := transport.PrepareRequest(t.Context(), MethodServerDiscover, DiscoverParams{
		Meta: requestMetaForContract(),
	})
	require.NoError(t, probeErr)
	require.NoError(t, probe.Deliver())
	probeResult, probeErr := probe.Await(t.Context())

	// Assert.
	var rpcErr *RPCError
	require.ErrorAs(t, listenErr, &rpcErr)
	require.NoError(t, probeErr)
	require.JSONEq(t, `{"alive":true}`, string(probeResult))
}
