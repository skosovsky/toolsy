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

func TestStdio_RequestDrivenServerReceivesInitializeBeforeWriting(t *testing.T) {
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
	pending, err := transport.Request(
		context.Background(),
		MethodInitialize,
		validInitializeParams(),
	)
	require.NoError(t, err)
	result, err := pending.Await(ctx)

	// Assert.
	require.NoError(t, err)
	var initialized InitializeResult
	require.NoError(t, json.Unmarshal(result, &initialized))
	require.Equal(t, ProtocolVersion, initialized.ProtocolVersion)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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
	params := map[string]string{"payload": strings.Repeat("x", 4<<20)}
	pending, err := transport.Request(ctx, "large", params)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return transport.activeWrites.Load() > 0
	}, 2*time.Second, time.Millisecond)

	// Act.
	cancel()
	_, err = pending.Await(ctx)

	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.NotEmpty(t, pending.ID())
	select {
	case <-transport.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked pipe cancellation did not terminate and reap the session")
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
	active, err := transport.Request(
		activeCtx,
		"active",
		map[string]string{"payload": strings.Repeat("x", 4<<20)},
	)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return transport.activeWrites.Load() == 1
	}, 2*time.Second, time.Millisecond)
	queuedCtx, cancelQueued := context.WithCancel(context.Background())
	queued, err := transport.Request(queuedCtx, "queued", struct{}{})
	require.NoError(t, err)
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
	healthy, err := transport.Request(context.Background(), "healthy", struct{}{})
	require.NoError(t, err)
	cancelActive()
	_, activeErr := active.Await(activeCtx)
	require.ErrorIs(t, activeErr, context.Canceled)
	healthyCtx, cancelHealthy := context.WithTimeout(context.Background(), time.Second)
	defer cancelHealthy()
	_, healthyErr := healthy.Await(healthyCtx)
	var crash *TransportCrashError
	require.ErrorAs(t, healthyErr, &crash)
	require.ErrorIs(t, healthyErr, context.Canceled)
	select {
	case <-transport.processDone:
	case <-time.After(5 * time.Second):
		t.Fatal("active write cancellation did not terminate the blocked process")
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
	transport.abortWrite(write, context.Canceled)

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

func TestStdio_InvalidUTF8DoesNotCorruptCorrelation(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "invalid-utf8-then-response"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(
		context.Background(),
		MethodInitialize,
		validInitializeParams(),
	)
	require.NoError(t, err)

	// Act.
	result, err := pending.Await(context.Background())

	// Assert.
	require.NoError(t, err)
	require.NotEmpty(t, result)
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
			pending, err := transport.Request(context.Background(), "test/malformed", struct{}{})
			require.NoError(t, err)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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
	pending, err := transport.Request(context.Background(), "test", nil)
	require.NoError(t, err)
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

func TestStdio_MalformedIncomingRequestReceivesInvalidRequest(t *testing.T) {
	// Arrange.
	transport := NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess", "--", "malformed-incoming-request"},
	)
	require.NoError(t, transport.Start(context.Background()))
	pending, err := transport.Request(context.Background(), MethodInitialize, validInitializeParams())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Act.
	result, err := pending.Await(ctx)

	// Assert.
	require.NoError(t, err)
	require.NotEmpty(t, result)
	require.NoError(t, transport.Close())
}

func TestStdioHelperProcess(_ *testing.T) {
	// Arrange.
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

	// Act.
	exitCode := runStdioHelperProcess(mode)

	// Assert.
	// Parent transport tests observe the wire output; a non-zero helper exit
	// makes protocol/setup failures deterministic at the process boundary.
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func runStdioHelperProcess(mode string) int {
	if handleStdioHelperBeforeRead(mode) {
		return 0
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return 2
	}
	if handleStdioHelperAfterRead(mode) {
		return 0
	}
	var request Request
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		return 3
	}
	if writeMalformedStdioHelperResponse(mode, request.ID) {
		return 0
	}
	if mode == "descendant-process-tree" {
		child := newLongLivedDescendant()
		if err := child.Start(); err != nil {
			return 12
		}
		response := Response{
			JSONRPC: JSONRPCVersion,
			ID:      request.ID,
			Result:  fmt.Appendf(nil, `{"pid":%d}`, child.Process.Pid),
		}
		body, err := json.Marshal(response)
		if err != nil {
			return 13
		}
		fmt.Println(string(body))
		time.Sleep(30 * time.Second)
		return 0
	}
	exerciseStdioIncomingProtocolError(mode, scanner)
	if request.Method != MethodInitialize ||
		!strings.Contains(string(request.Params), ProtocolVersion) {
		return 4
	}
	result := initializeResult(ServerCapabilities{})
	response := Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: result}
	body, err := json.Marshal(response)
	if err != nil {
		return 5
	}
	fmt.Println(string(body))
	return 0
}

func exerciseStdioIncomingProtocolError(mode string, scanner *bufio.Scanner) {
	var payload []byte
	var wantCode JSONNumber
	switch mode {
	case "invalid-utf8-then-response":
		payload = []byte{0xff, '\n'}
		wantCode = JSONRPCParseError
	case "malformed-incoming-request":
		payload = []byte(`{"jsonrpc":"1.0","id":"server-invalid","method":"roots/list"}` + "\n")
		wantCode = JSONRPCInvalidRequest
	default:
		return
	}
	if _, err := os.Stdout.Write(payload); err != nil || !scanner.Scan() {
		os.Exit(8)
	}
	var protocolError Response
	if err := json.Unmarshal(scanner.Bytes(), &protocolError); err != nil ||
		protocolError.Error == nil || protocolError.Error.Code != wantCode {
		os.Exit(9)
	}
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
