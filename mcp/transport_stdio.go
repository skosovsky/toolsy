//nolint:exhaustruct // Transport options deliberately initialize only configured runtime state.
package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/toolkits/httptool"
)

const (
	rpcJSONLineScannerMaxBytes = 1024 * 1024
	maxStdioLogLineBytes       = 256
	stdioStderrReadBufferBytes = 4096
	stdioWriteQueueSize        = 64
	stdioProcessExitGrace      = 50 * time.Millisecond
)

type StdioTransportOption func(*StdioTransport)

type stdioWrite struct {
	ctx    context.Context
	body   []byte
	result chan error
	onSent func()
	state  atomic.Int32
}

type startedStdioProcess struct {
	cmd    *exec.Cmd
	tree   processTree
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

const (
	stdioWriteQueued int32 = iota
	stdioWriteActive
	stdioWriteCancelled
	stdioWriteFinished
)

func WithLogger(logger *slog.Logger) StdioTransportOption {
	return func(transport *StdioTransport) {
		if logger != nil {
			transport.logger = logger
		}
	}
}

// WithStdioMaxStreamBytes limits protocol bytes read from child stdout.
// Stderr is consumed independently with bounded per-line logging.
func WithStdioMaxStreamBytes(limit int) StdioTransportOption {
	return func(transport *StdioTransport) {
		if limit > 0 {
			transport.maxStreamBytes = limit
		}
	}
}

type StdioTransport struct {
	executable string
	args       []string
	logger     *slog.Logger

	maxStreamBytes int
	mu             sync.Mutex
	started        bool
	closed         bool
	lifetimeCtx    context.Context
	cancel         context.CancelFunc
	cmd            *exec.Cmd
	processTree    processTree
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	readerDone     chan struct{}
	writerDone     chan struct{}
	stderrDone     chan struct{}
	processDone    chan struct{}
	processErr     chan error
	processCause   error
	writeQueue     chan *stdioWrite
	activeWrites   atomic.Int32
	peer           *rpcPeer
	terminalErr    error
	requestHandler RequestHandler
	notifyHandlers map[string]NotificationHandler
	closeOnce      sync.Once
}

func NewStdioTransport(
	executable string,
	args []string,
	opts ...StdioTransportOption,
) *StdioTransport {
	transport := &StdioTransport{
		executable:     executable,
		args:           append([]string(nil), args...),
		logger:         slog.Default(),
		maxStreamBytes: httptool.DefaultMaxSSEStreamBytes,
		readerDone:     make(chan struct{}),
		writerDone:     make(chan struct{}),
		stderrDone:     make(chan struct{}),
		processDone:    make(chan struct{}),
		processErr:     make(chan error, 1),
		writeQueue:     make(chan *stdioWrite, stdioWriteQueueSize),
		notifyHandlers: make(map[string]NotificationHandler),
	}
	for _, opt := range opts {
		opt(transport)
	}
	return transport
}

func (t *StdioTransport) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrTransportClosed
	}
	if t.started {
		return nil
	}

	lifetimeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	process, err := t.startProcess(lifetimeCtx)
	if err != nil {
		cancel()
		return err
	}

	t.lifetimeCtx = lifetimeCtx
	t.cancel = cancel
	t.cmd = process.cmd
	t.processTree = process.tree
	t.stdin = process.stdin
	t.stdout = httptool.LimitStreamReadCloserWithContext(lifetimeCtx, process.stdout, t.maxStreamBytes)
	t.stderr = process.stderr
	t.peer = newRPCPeer(lifetimeCtx, t.logger, t.send)
	t.peer.setRequestHandler(t.requestHandler)
	for method, handler := range t.notifyHandlers {
		t.peer.setNotificationHandler(method, handler)
	}
	t.started = true
	go t.forwardStderr(lifetimeCtx, process.stderr)
	go t.writeLoop(lifetimeCtx, process.stdin)
	go t.readLoop()
	go t.waitProcess(process.cmd)
	return nil
}

func (t *StdioTransport) startProcess(ctx context.Context) (startedStdioProcess, error) {
	// #nosec G204 -- the executable and arguments are explicitly supplied by the caller.
	//nolint:gosec // Caller explicitly supplies the executable contract.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), t.executable, t.args...)
	configureProcessTree(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return startedStdioProcess{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return startedStdioProcess{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		closeStdioPipes(stdin, stdout)
		return startedStdioProcess{}, err
	}
	if startErr := cmd.Start(); startErr != nil {
		closeStdioPipes(stdin, stdout, stderr)
		return startedStdioProcess{}, startErr
	}
	tree, err := attachProcessTree(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		closeStdioPipes(stdin, stdout, stderr)
		return startedStdioProcess{}, fmt.Errorf("mcp: attach stdio process tree: %w", err)
	}
	return startedStdioProcess{cmd: cmd, tree: tree, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

func (t *StdioTransport) waitProcess(cmd *exec.Cmd) {
	state, err := cmd.Process.Wait()
	if err == nil && !state.Success() {
		err = &exec.ExitError{ProcessState: state}
	}
	t.mu.Lock()
	t.processCause = err
	t.mu.Unlock()
	t.processErr <- err
	close(t.processDone)
	if t.lifetimeCtx.Err() != nil {
		return
	}
	if err == nil {
		err = errors.New("process exited")
	}
	t.closeAsync(&TransportCrashError{Transport: "stdio process", Err: err})
}

func (t *StdioTransport) send(ctx context.Context, body []byte) error {
	return t.sendTracked(ctx, body, nil)
}

func (t *StdioTransport) sendTracked(ctx context.Context, body []byte, onSent func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	if !t.started || t.closed || t.stdin == nil {
		terminalErr := t.terminalErr
		t.mu.Unlock()
		if terminalErr != nil {
			return terminalErr
		}
		return ErrTransportClosed
	}
	t.mu.Unlock()
	write := &stdioWrite{
		ctx:    ctx,
		body:   append(append([]byte(nil), body...), '\n'),
		result: make(chan error, 1),
		onSent: onSent,
	}
	select {
	case t.writeQueue <- write:
	case <-ctx.Done():
		return ctx.Err()
	case <-t.lifetimeCtx.Done():
		return t.failureCause(ErrTransportClosed)
	}
	select {
	case err := <-write.result:
		return err
	case <-ctx.Done():
		t.abortWrite(write, ctx.Err())
		return ctx.Err()
	case <-t.lifetimeCtx.Done():
		return t.failureCause(ErrTransportClosed)
	}
}

func (t *StdioTransport) writeLoop(ctx context.Context, stdin io.Writer) {
	defer close(t.writerDone)
	for {
		select {
		case <-ctx.Done():
			return
		case write := <-t.writeQueue:
			if !write.state.CompareAndSwap(stdioWriteQueued, stdioWriteActive) {
				writeErr := write.ctx.Err()
				if writeErr == nil {
					writeErr = context.Canceled
				}
				write.result <- writeErr
				continue
			}
			t.activeWrites.Add(1)
			if err := write.ctx.Err(); err != nil {
				write.state.Store(stdioWriteCancelled)
				t.activeWrites.Add(-1)
				write.result <- err
				continue
			}
			_, err := stdin.Write(write.body)
			write.state.CompareAndSwap(stdioWriteActive, stdioWriteFinished)
			t.activeWrites.Add(-1)
			if err == nil && write.onSent != nil {
				write.onSent()
			}
			write.result <- err
			if err != nil {
				t.closeAsync(&TransportCrashError{Transport: "stdio write", Err: err})
				return
			}
		}
	}
}

func (t *StdioTransport) abortWrite(write *stdioWrite, cause error) {
	for {
		switch write.state.Load() {
		case stdioWriteQueued:
			if write.state.CompareAndSwap(stdioWriteQueued, stdioWriteCancelled) {
				return
			}
		case stdioWriteActive:
			if write.state.CompareAndSwap(stdioWriteActive, stdioWriteCancelled) {
				t.closeAsync(&TransportCrashError{Transport: "stdio write cancellation", Err: cause})
				return
			}
		case stdioWriteCancelled, stdioWriteFinished:
			return
		}
	}
}

func (t *StdioTransport) Request(
	ctx context.Context,
	method string,
	params any,
) (PendingRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.mu.Lock()
	peer := t.peer
	started := t.started && !t.closed
	t.mu.Unlock()
	if !started || peer == nil {
		return nil, ErrTransportClosed
	}
	pending, body, err := peer.beginRequest(method, params)
	if err != nil {
		return nil, err
	}
	go func() {
		sendCtx, stopSend := contextUntilPendingTerminal(ctx, pending.terminal)
		defer stopSend()
		defer pending.finishDelivery()
		if sendErr := t.sendTracked(sendCtx, body, pending.markSent); sendErr != nil {
			peer.failPending(pending, t.failureCause(sendErr))
		}
	}()
	return pending, nil
}

func (t *StdioTransport) Notify(ctx context.Context, method string, params any) error {
	t.mu.Lock()
	peer := t.peer
	started := t.started && !t.closed
	t.mu.Unlock()
	if peer == nil || !started {
		return ErrTransportClosed
	}
	return peer.notifyMessage(ctx, method, params)
}

func (t *StdioTransport) OnRequest(handler RequestHandler) {
	t.mu.Lock()
	t.requestHandler = handler
	peer := t.peer
	t.mu.Unlock()
	if peer != nil {
		peer.setRequestHandler(handler)
	}
}

func (t *StdioTransport) OnNotification(method string, handler NotificationHandler) {
	t.mu.Lock()
	if handler == nil {
		delete(t.notifyHandlers, method)
	} else {
		t.notifyHandlers[method] = handler
	}
	peer := t.peer
	t.mu.Unlock()
	if peer != nil {
		peer.setNotificationHandler(method, handler)
	}
}

func (t *StdioTransport) readLoop() {
	defer close(t.readerDone)
	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(nil, rpcJSONLineScannerMaxBytes)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		if err := t.peer.dispatch(line); err != nil {
			t.logger.Warn("mcp: invalid stdio message", "err", err, "bytes", len(line))
		}
	}
	err := scanner.Err()
	if err == nil {
		err = errors.New("process stdout closed")
	}
	if t.lifetimeCtx.Err() != nil {
		t.peer.close(ErrTransportClosed)
		return
	}
	if scanner.Err() == nil {
		select {
		case <-t.processDone:
			t.closeAsync(t.processCrashError())
			return
		case <-time.After(stdioProcessExitGrace):
		}
	}
	t.closeAsync(&TransportCrashError{Transport: "stdio", Err: err})
}

func (t *StdioTransport) processCrashError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cause := t.processCause
	if cause == nil {
		cause = errors.New("process exited")
	}
	return &TransportCrashError{Transport: "stdio process", Err: cause}
}

func (t *StdioTransport) forwardStderr(ctx context.Context, reader io.Reader) {
	defer close(t.stderrDone)
	buffered := bufio.NewReaderSize(reader, stdioStderrReadBufferBytes)
	line := make([]byte, 0, maxStdioLogLineBytes)
	truncated := false
	for {
		fragment, isPrefix, err := buffered.ReadLine()
		remaining := maxStdioLogLineBytes - len(line)
		if remaining > 0 {
			line = append(line, fragment[:min(len(fragment), remaining)]...)
		}
		truncated = truncated || isPrefix || len(fragment) > remaining
		if !isPrefix && (err == nil || len(line) > 0) {
			t.logger.InfoContext(ctx, "mcp stderr", "line", formatStderrLine(line, truncated))
			line = line[:0]
			truncated = false
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil && !errors.Is(err, context.Canceled) {
				t.logger.WarnContext(ctx, "mcp stderr read", "err", err)
			}
			return
		}
	}
}

func (t *StdioTransport) closeAsync(cause error) {
	go func() {
		_ = t.closeWithCause(cause)
	}()
}

func formatStderrLine(line []byte, truncated bool) string {
	value := strings.ToValidUTF8(string(line), "�")
	if len(value) <= maxStdioLogLineBytes && !truncated {
		return value
	}
	const suffix = "...(truncated)"
	limit := maxStdioLogLineBytes - len(suffix)
	if len(value) > limit {
		value = value[:limit]
		for len(value) > 0 && !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value + suffix
}

func (t *StdioTransport) MaxStreamBytes() int { return t.maxStreamBytes }

func (t *StdioTransport) failureCause(fallback error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.terminalErr != nil {
		return t.terminalErr
	}
	return fallback
}

func (t *StdioTransport) Close() error {
	return t.closeWithCause(ErrTransportClosed)
}

func (t *StdioTransport) closeWithCause(cause error) error {
	var closeErr error
	t.closeOnce.Do(func() {
		t.mu.Lock()
		if !t.started {
			t.closed = true
			t.terminalErr = cause
			t.mu.Unlock()
			return
		}
		t.terminalErr = cause
		t.closed = true
		cancel := t.cancel
		stdin := t.stdin
		stdout := t.stdout
		stderr := t.stderr
		cmd := t.cmd
		tree := t.processTree
		peer := t.peer
		t.mu.Unlock()
		if peer != nil {
			peer.close(cause)
		}
		if cancel != nil {
			cancel()
		}
		closeStdioPipes(stdin, stdout, stderr)
		closeErr = t.stopProcess(cmd, tree)
		<-t.writerDone
		<-t.readerDone
		<-t.stderrDone
	})
	return closeErr
}

func closeStdioPipes(pipes ...io.Closer) {
	for _, pipe := range pipes {
		if pipe != nil {
			_ = pipe.Close()
		}
	}
}

func (t *StdioTransport) stopProcess(cmd *exec.Cmd, tree processTree) error {
	if cmd == nil {
		return nil
	}
	if cmd.Process != nil {
		_ = killProcessTree(tree, cmd)
	}
	<-t.processDone
	err := <-t.processErr
	if err != nil && t.lifetimeCtx.Err() != nil {
		err = nil
	}
	return err
}

var _ Transport = (*StdioTransport)(nil)
var _ StreamByteCapTransport = (*StdioTransport)(nil)
