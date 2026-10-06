package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingWriter struct{ calls int }

func (w *recordingWriter) Write(body []byte) (int, error) {
	w.calls++
	return len(body), nil
}

func TestStdio_StderrVolumeAndLongLinesNeverCrashTransport(t *testing.T) {
	// Arrange.
	logger := slog.New(slog.DiscardHandler)
	transport := NewStdioTransport("unused", nil, WithLogger(logger))
	stderr := strings.NewReader(strings.Repeat("x", 2*rpcJSONLineScannerMaxBytes) + "\nnext\n")

	// Act.
	transport.forwardStderr(context.Background(), stderr)

	// Assert.
	transport.mu.Lock()
	closed := transport.closed
	terminalErr := transport.terminalErr
	transport.mu.Unlock()
	require.False(t, closed)
	require.NoError(t, terminalErr)
}

func TestStdio_RequestDrivenServerReceivesDiscoveryBeforeWriting(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "request-response"},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	startCtx, cancelStart := context.WithCancel(context.Background())
	require.NoError(t, transport.Start(startCtx))
	cancelStart()

	// Act.
	pending, err := transport.PrepareRequest(
		context.Background(),
		MethodServerDiscover,
		migrationDiscoveryParams(),
	)
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	result, err := pending.Await(ctx)

	// Assert.
	require.NoError(t, err)
	var initialized DiscoverResult
	require.NoError(t, json.Unmarshal(result, &initialized))
	require.Contains(t, initialized.SupportedVersions, ProtocolVersion)
	require.NoError(t, transport.Close())
	select {
	case <-transport.stderrDone:
	default:
		t.Fatal("stderr forwarder and parent pipe were not closed")
	}
}

func TestStdio_ProcessExitUnblocksPendingRequest(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "exit-without-response"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	require.Error(t, err)
	select {
	case <-transport.processDone:
	case <-time.After(time.Second):
		t.Fatal("exited child was not reaped without external Close")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_NonzeroProcessExitWinsStdoutEOFRace(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "exit-nonzero"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	var crash *TransportCrashError
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &crash)
	require.ErrorAs(t, err, &exitErr)
	require.NoError(t, transport.Close())
}

func TestStdio_ClosedStdoutTerminatesAndReapsLiveProcess(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "close-stdout-then-sleep"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	require.Error(t, err)
	select {
	case <-transport.processDone:
	case <-time.After(time.Second):
		t.Fatal("stdout closure did not terminate and reap the live child")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_BlockedWriteHonorsContextCancellation(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "never-read"},
	)
	require.NoError(t, transport.Start(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	params := map[string]any{"_meta": migrationDiscoveryParams().Meta, "payload": strings.Repeat("x", 4<<20)}
	pending, err := transport.PrepareRequest(ctx, "large", params)
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	require.Eventually(t, func() bool {
		return transport.activeWrites.Load() > 0
	}, 2*time.Second, time.Millisecond)

	// Act.
	cancel()
	_, err = pending.Await(ctx)

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.NotEmpty(t, pending.ID())
	// Cancellation is not permission to kill unrelated requests. An active pipe
	// write keeps its writer verdict; explicit Close owns process-tree cleanup.
	transport.mu.Lock()
	closed := transport.closed
	transport.mu.Unlock()
	require.False(t, closed)
	require.NoError(t, transport.Close())
	select {
	case <-transport.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("explicit Close did not terminate and reap the blocked process")
	}
	select {
	case <-transport.writerDone:
	default:
		t.Fatal("stdio writer leaked after blocked write cancellation")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_CancellingQueuedWriteDoesNotAbortActiveWrite(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "never-read"},
	)
	require.NoError(t, transport.Start(context.Background()))
	activeCtx, cancelActive := context.WithCancel(context.Background())
	active, err := transport.PrepareRequest(
		activeCtx,
		"active",
		map[string]any{"_meta": migrationDiscoveryParams().Meta, "payload": strings.Repeat("x", 4<<20)},
	)
	require.NoError(t, err)
	require.NoError(t, active.Deliver())
	require.Eventually(t, func() bool {
		return transport.activeWrites.Load() == 1
	}, 2*time.Second, time.Millisecond)
	queuedCtx, cancelQueued := context.WithCancel(context.Background())
	queued, err := transport.PrepareRequest(queuedCtx, "queued", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, queued.Deliver())
	require.Eventually(t, func() bool {
		return len(transport.writeQueue) == 1
	}, time.Second, time.Millisecond)
	cancelQueued()

	// Act.
	_, queuedErr := queued.Await(queuedCtx)

	// Assert.
	require.ErrorIs(t, queuedErr, context.Canceled)
	select {
	case <-transport.processDone:
		t.Fatal("queued cancellation terminated another request's active write")
	case <-time.After(100 * time.Millisecond):
	}
	healthy, err := transport.PrepareRequest(context.Background(), "healthy", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, healthy.Deliver())
	cancelActive()
	_, activeErr := active.Await(activeCtx)
	require.ErrorIs(t, activeErr, context.Canceled)
	transport.mu.Lock()
	closed := transport.closed
	transport.mu.Unlock()
	require.False(t, closed, "active cancellation must not terminate unrelated requests")
	require.NoError(t, transport.Close())
	healthyCtx, cancelHealthy := context.WithTimeout(context.Background(), time.Second)
	defer cancelHealthy()
	_, healthyErr := healthy.Await(healthyCtx)
	require.ErrorIs(t, healthyErr, ErrTransportClosed)
	select {
	case <-transport.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("explicit Close did not terminate the blocked process")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_WriteLoopRechecksCancellationAfterActivation(t *testing.T) {
	// Arrange.
	transport := &StdioTransport{
		writeQueue: make(chan *stdioWrite, 1),
		writerDone: make(chan struct{}),
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(context.Background())
	writer := &recordingWriter{}
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()
	write := &stdioWrite{
		ctx:    requestCtx,
		body:   []byte("request"),
		result: make(chan error, 1),
	}
	go transport.writeLoop(lifetimeCtx, writer)

	// Act.
	transport.writeQueue <- write
	err := <-write.result
	cancelLifetime()
	<-transport.writerDone

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, writer.calls)
	require.Equal(t, stdioWriteCancelled, write.state.Load())
}

func TestStdio_SuccessfulWriteIsFinishedBeforeDeliveryCallback(t *testing.T) {
	// Arrange.
	transport := &StdioTransport{
		writeQueue: make(chan *stdioWrite, 1),
		writerDone: make(chan struct{}),
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(context.Background())
	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	write := &stdioWrite{
		ctx:    context.Background(),
		body:   []byte("request"),
		result: make(chan error, 1),
		onSent: func() {
			close(callbackStarted)
			<-releaseCallback
		},
	}
	go transport.writeLoop(lifetimeCtx, &recordingWriter{})
	transport.writeQueue <- write
	<-callbackStarted

	// Act.
	transport.abortWrite(write)

	// Assert.
	require.Equal(t, stdioWriteFinished, write.state.Load())
	require.Never(t, func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return transport.closed
	}, 100*time.Millisecond, time.Millisecond)
	close(releaseCallback)
	require.NoError(t, <-write.result)
	cancelLifetime()
	<-transport.writerDone
}

func TestStdio_InvalidUTF8FailsClosedWithoutReply(t *testing.T) {
	// Arrange.
	assertStdioInputProducesNoReply(t, []byte{0xff}, "payload is not valid UTF-8")
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "invalid-utf8-then-response"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(
		context.Background(),
		MethodServerDiscover,
		migrationDiscoveryParams(),
	)
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())

	// Act.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := pending.Await(ctx)

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, "payload is not valid UTF-8")
	require.Empty(t, result)
	require.NoError(t, transport.Close())
}

func TestStdio_MalformedCorrelatedResponseUnblocksPendingRequest(t *testing.T) {
	for _, mode := range []string{
		"malformed-response-then-sleep",
		"malformed-typed-response-then-sleep",
		"wrong-version-response-then-sleep",
		"missing-version-response-then-sleep",
	} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			transport := NewStdioTransport(
				os.Args[0],
				[]string{"-test.run=TestStdioHelperProcess", "--", mode},
			)
			require.NoError(t, transport.Start(context.Background()))
			pending, err := transport.PrepareRequest(context.Background(), "test/malformed", migrationDiscoveryParams())
			require.NoError(t, err)
			require.NoError(t, pending.Deliver())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			// Act.
			_, err = pending.Await(ctx)

			// Assert.
			var invalid *InvalidPayloadError
			require.ErrorAs(t, err, &invalid)
			require.NoError(t, transport.Close())
		})
	}
}

func TestStdio_ProcessExitDetectedWhileDescendantHoldsStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX inherited-descriptor fixture")
	}
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "exit-with-descendant-stdout"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	var crash *TransportCrashError
	require.ErrorAs(t, err, &crash)
	select {
	case <-transport.processDone:
	case <-time.After(time.Second):
		t.Fatal("exited parent was not independently reaped")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_ProcessExitDetectedWhileDescendantHoldsStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX inherited-descriptor fixture")
	}
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "exit-with-descendant-stderr"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	// Act.
	_, err = pending.Await(ctx)

	// Assert.
	var crash *TransportCrashError
	require.ErrorAs(t, err, &crash)
	select {
	case <-transport.processDone:
	case <-time.After(time.Second):
		t.Fatal("exited parent was not independently reaped")
	}
	require.NoError(t, transport.Close())
}

func TestStdio_CloseKillsDescendantProcessTree(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "descendant-process-tree"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), "test", migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	result, err := pending.Await(context.Background())
	require.NoError(t, err)
	var child struct {
		PID int `json:"pid"`
	}
	require.NoError(t, json.Unmarshal(result, &child))
	require.True(t, processExists(child.PID))

	// Act.
	closeErr := transport.Close()

	// Assert.
	require.NoError(t, closeErr)
	require.Eventually(t, func() bool {
		return !processExists(child.PID)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestStdio_MalformedIncomingRequestFailsClosedWithoutReply(t *testing.T) {
	// Arrange.
	assertStdioInputProducesNoReply(
		t,
		[]byte(`{"jsonrpc":"1.0","id":"server-invalid","method":"roots/list"}`),
		`invalid jsonrpc version "1.0"`,
	)
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "malformed-incoming-request"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.PrepareRequest(context.Background(), MethodServerDiscover, migrationDiscoveryParams())
	require.NoError(t, err)
	require.NoError(t, pending.Deliver())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Act.
	result, err := pending.Await(ctx)

	// Assert.
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, `invalid jsonrpc version "1.0"`)
	require.Empty(t, result)
	require.NoError(t, transport.Close())
}

func assertStdioInputProducesNoReply(t *testing.T, payload []byte, cause string) {
	t.Helper()
	var sent [][]byte
	peer := newRPCPeer(context.Background(), nil, func(_ context.Context, body []byte) error {
		sent = append(sent, append([]byte(nil), body...))
		return nil
	})
	t.Cleanup(func() { peer.close(ErrTransportClosed) })
	err := peer.dispatch(payload)
	var invalid *InvalidPayloadError
	require.ErrorAs(t, err, &invalid)
	require.ErrorContains(t, err, cause)
	require.Empty(t, sent, "invalid input must not cause an unsolicited protocol reply")
}

func TestStdioHelperProcess(_ *testing.T) {
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	mode := os.Args[separator+1]
	if handleStdioHelperBeforeRead(mode) {
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	if handleStdioHelperAfterRead(mode) {
		os.Exit(0)
	}
	var request Request
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		os.Exit(3)
	}
	if writeMalformedStdioHelperResponse(mode, request.ID) {
		return
	}
	if mode == "descendant-process-tree" {
		child := newLongLivedDescendant()
		if err := child.Start(); err != nil {
			os.Exit(12)
		}
		response := Response{
			JSONRPC: JSONRPCVersion,
			ID:      request.ID,
			Result:  fmt.Appendf(nil, `{"pid":%d}`, child.Process.Pid),
		}
		body, err := json.Marshal(response)
		if err != nil {
			os.Exit(13)
		}
		fmt.Println(string(body))
		time.Sleep(30 * time.Second)
		return
	}
	if exerciseStdioIncomingProtocolError(mode, scanner) {
		os.Exit(0)
	}
	if request.Method != MethodServerDiscover ||
		!strings.Contains(string(request.Params), ProtocolVersion) {
		os.Exit(4)
	}
	result := completeDiscovery(ServerCapabilities{})
	response := Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: result}
	body, err := json.Marshal(response)
	if err != nil {
		os.Exit(5)
	}
	fmt.Println(string(body))
	os.Exit(0)
}

func exerciseStdioIncomingProtocolError(mode string, scanner *bufio.Scanner) bool {
	var payload []byte
	switch mode {
	case "invalid-utf8-then-response":
		payload = []byte{0xff, '\n'}
	case "malformed-incoming-request":
		payload = []byte(`{"jsonrpc":"1.0","id":"server-invalid","method":"roots/list"}` + "\n")
	default:
		return false
	}
	if _, err := os.Stdout.Write(payload); err != nil {
		os.Exit(8)
	}
	// The current client must close without replying to the server request.
	// Any received frame is a regression to the removed reply path.
	if scanner.Scan() {
		os.Exit(9)
	}
	return true
}

func handleStdioHelperBeforeRead(mode string) bool {
	switch mode {
	case "never-read":
		time.Sleep(10 * time.Second)
		return true
	case "long-lived-descendant":
		time.Sleep(30 * time.Second)
		return true
	default:
		return false
	}
}

func handleStdioHelperAfterRead(mode string) bool {
	switch mode {
	case "exit-without-response":
		return true
	case "exit-nonzero":
		os.Exit(7)
	case "close-stdout-then-sleep":
		_ = os.Stdout.Close()
		time.Sleep(10 * time.Second)
		return true
	case "exit-with-descendant-stdout":
		child := exec.Command("sh", "-c", "sleep 3") // #nosec G204 -- fixed test fixture.
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(6)
		}
		return true
	case "exit-with-descendant-stderr":
		child := exec.Command("sh", "-c", "sleep 3") // #nosec G204 -- fixed test fixture.
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(6)
		}
		return true
	}
	return false
}

func writeMalformedStdioHelperResponse(mode string, id json.RawMessage) bool {
	switch mode {
	case "malformed-response-then-sleep":
		fmt.Printf(
			`{"jsonrpc":"2.0","id":%s,"result":{},"error":{"code":1,"message":"bad"}}`+"\n",
			id,
		)
	case "malformed-typed-response-then-sleep":
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":"bad"}`+"\n", id)
	case "wrong-version-response-then-sleep":
		fmt.Printf(`{"jsonrpc":"1.0","id":%s,"result":{}}`+"\n", id)
	case "missing-version-response-then-sleep":
		fmt.Printf(`{"id":%s,"result":{}}`+"\n", id)
	default:
		return false
	}
	time.Sleep(10 * time.Second)
	return true
}
