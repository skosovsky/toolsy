package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// StdioTransportOption configures StdioTransport.
type StdioTransportOption func(*stdioTransport)

// WithLogger sets the logger for stderr of the child process. If not set, slog.Default() is used.
func WithLogger(logger *slog.Logger) StdioTransportOption {
	return func(t *stdioTransport) {
		if logger != nil {
			t.logger = logger
		}
	}
}

// WithStdioFirstLineTimeout sets the max time to wait for the first line from the child's stdout after start. If not set, stdioFirstLineTimeout (30s) is used.
func WithStdioFirstLineTimeout(d time.Duration) StdioTransportOption {
	return func(t *stdioTransport) {
		if d > 0 {
			t.firstLineTimeout = d
		}
	}
}

const stdioFirstLineTimeout = 30 * time.Second

var errStdioFirstLineTimeout = errors.New("mcp stdio: timeout waiting for first line from process stdout")

// callResult holds the result or error from a Call for the pending map.
type callResult struct {
	Result []byte
	Err    error
}

type stdioTransport struct {
	executable string
	args       []string
	logger     *slog.Logger

	firstLineTimeout time.Duration
	firstLineSeen    chan struct{}
	firstLineOnce    sync.Once

	startMu   sync.Mutex
	started   bool
	startErr  error
	cmd       *exec.Cmd
	cmdCancel context.CancelFunc
	stdin     io.WriteCloser
	stdout    io.ReadCloser

	requestID atomic.Uint64
	pending   sync.Map // id string -> chan *callResult

	readerDone chan struct{}
	writeMu    sync.Mutex

	notifyMu       sync.Mutex
	notifyHandlers map[string]func(params []byte)
}

// NewStdioTransport creates a transport that runs a child process and uses stdin/stdout for JSON-RPC.
// Stderr is forwarded to the logger. Use WithLogger to set a custom logger; otherwise slog.Default() is used.
func NewStdioTransport(executable string, args []string, opts ...StdioTransportOption) *StdioTransport {
	t := &stdioTransport{
		executable:       executable,
		args:             args,
		logger:           slog.Default(),
		firstLineTimeout: stdioFirstLineTimeout,
		firstLineSeen:    make(chan struct{}),
		readerDone:       make(chan struct{}),
		notifyHandlers:   make(map[string]func(params []byte)),
	}
	for _, opt := range opts {
		opt(t)
	}
	return &StdioTransport{impl: t}
}

// StdioTransport is the public type that implements Transport.
type StdioTransport struct {
	impl *stdioTransport
}

// Start starts the child process and the stdout read loop.
func (s *StdioTransport) Start(ctx context.Context) error {
	return s.impl.start(ctx)
}

func (t *stdioTransport) start(ctx context.Context) error {
	t.startMu.Lock()
	defer t.startMu.Unlock()
	if t.started {
		if t.startErr != nil {
			return t.startErr
		}
		return nil
	}
	t.started = true

	ctxCmd, cancel := context.WithCancel(ctx)
	t.cmdCancel = cancel
	// #nosec G204 -- executable and args come from caller (NewStdioTransport); caller is responsible for trust.
	t.cmd = exec.CommandContext(ctxCmd, t.executable, t.args...)

	stdinPipe, err := t.cmd.StdinPipe()
	if err != nil {
		t.startErr = err
		return err
	}
	t.stdin = stdinPipe

	stdoutPipe, err := t.cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		t.startErr = err
		return err
	}
	t.stdout = stdoutPipe

	stderrPipe, err := t.cmd.StderrPipe()
	if err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		t.startErr = err
		return err
	}

	if err := t.cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		t.startErr = err
		return err
	}

	// Forward stderr to logger so JSON-RPC channel is not broken.
	go t.forwardStderr(stderrPipe)

	// Read loop: parse JSON-RPC from stdout and dispatch.
	go t.readLoop()

	timeout := t.firstLineTimeout
	if timeout <= 0 {
		timeout = stdioFirstLineTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.cmdCancel()
		_ = t.stdin.Close()
		<-t.readerDone
		t.startErr = ctx.Err()
		return ctx.Err()
	case <-t.firstLineSeen:
		return nil
	case <-timer.C:
		t.cmdCancel()
		_ = t.stdin.Close()
		<-t.readerDone
		t.startErr = errStdioFirstLineTimeout
		return errStdioFirstLineTimeout
	}
}

func (t *stdioTransport) forwardStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		t.logger.Info("mcp stderr", "line", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.logger.Error("mcp stderr read", "err", err)
	}
}

func (t *stdioTransport) readLoop() {
	defer close(t.readerDone)

	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		t.firstLineOnce.Do(func() { close(t.firstLineSeen) })
		if len(line) == 0 {
			continue
		}
		t.dispatchMessage(line)
	}

	// Process exited or stdout closed: unblock all pending Call.
	err := scanner.Err()
	if err == nil {
		err = fmt.Errorf("process stdout closed")
	}
	t.unblockAllPending(err)
}

func (t *stdioTransport) dispatchMessage(line []byte) {
	var raw struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *JSONRPCError   `json:"error"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		t.logger.Info("mcp invalid json from stdout", "line", string(line), "err", err)
		return
	}

	// If "method" is set and "result"/"error" absent, it's a notification.
	if raw.Method != "" && raw.Result == nil && raw.Error == nil {
		t.notifyMu.Lock()
		handler := t.notifyHandlers[raw.Method]
		t.notifyMu.Unlock()
		if handler != nil {
			handler(raw.Params)
		}
		return
	}

	// Response: has id and result or error.
	if len(raw.ID) == 0 || (raw.Result == nil && raw.Error == nil) {
		return
	}
	var idAny any
	if err := json.Unmarshal(raw.ID, &idAny); err != nil {
		return
	}
	id := fmt.Sprint(idAny)
	if chVal, ok := t.pending.LoadAndDelete(id); ok {
		ch := chVal.(chan *callResult)
		res := &callResult{}
		if raw.Error != nil {
			res.Err = fmt.Errorf("json-rpc error %d: %s", raw.Error.Code, raw.Error.Message)
		} else {
			res.Result = raw.Result
		}
		select {
		case ch <- res:
		default:
			// Caller already gave up (context cancelled).
		}
		close(ch)
	}
}

func (t *stdioTransport) unblockAllPending(err error) {
	t.pending.Range(func(key, value any) bool {
		t.pending.Delete(key)
		ch := value.(chan *callResult)
		select {
		case ch <- &callResult{Err: err}:
		default:
		}
		close(ch)
		return true
	})
}

// Call sends a request and waits for the response.
func (s *StdioTransport) Call(ctx context.Context, method string, params any) ([]byte, string, error) {
	return s.impl.call(ctx, method, params)
}

func (t *stdioTransport) call(ctx context.Context, method string, params any) ([]byte, string, error) {
	t.startMu.Lock()
	if !t.started || t.startErr != nil {
		t.startMu.Unlock()
		return nil, "", fmt.Errorf("transport not started or failed: %w", t.startErr)
	}
	t.startMu.Unlock()

	id := t.requestID.Add(1)
	idStr := fmt.Sprintf("%d", id)

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return nil, "", err
		}
	}

	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(idStr),
		Method:  method,
		Params:  paramsRaw,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, "", err
	}

	ch := make(chan *callResult, 1)
	t.pending.Store(idStr, ch)
	defer t.pending.Delete(idStr)

	t.writeMu.Lock()
	_, err = t.stdin.Write(append(body, '\n'))
	t.writeMu.Unlock()
	if err != nil {
		return nil, "", err
	}

	select {
	case <-ctx.Done():
		return nil, idStr, ctx.Err()
	case res, ok := <-ch:
		if !ok {
			return nil, idStr, fmt.Errorf("response channel closed")
		}
		if res.Err != nil {
			return nil, idStr, res.Err
		}
		return res.Result, idStr, nil
	}
}

// Notify sends a notification (no response expected).
func (s *StdioTransport) Notify(ctx context.Context, method string, params any) error {
	return s.impl.notify(ctx, method, params)
}

func (t *stdioTransport) notify(_ context.Context, method string, params any) error {
	t.startMu.Lock()
	if !t.started || t.startErr != nil {
		t.startMu.Unlock()
		return fmt.Errorf("transport not started or failed: %w", t.startErr)
	}
	t.startMu.Unlock()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}
	notif := Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsRaw,
	}
	body, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	t.writeMu.Lock()
	_, err = t.stdin.Write(append(body, '\n'))
	t.writeMu.Unlock()
	return err
}

// OnNotification registers a handler for incoming notifications with the given method.
func (s *StdioTransport) OnNotification(method string, handler func(params []byte)) {
	s.impl.notifyMu.Lock()
	defer s.impl.notifyMu.Unlock()
	s.impl.notifyHandlers[method] = handler
}

// Close shuts down the transport and the child process.
func (s *StdioTransport) Close() error {
	return s.impl.close()
}

func (t *stdioTransport) close() error {
	t.startMu.Lock()
	if !t.started {
		t.startMu.Unlock()
		return nil
	}
	t.started = false
	t.startMu.Unlock()

	if t.cmdCancel != nil {
		t.cmdCancel()
	}
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.stdout != nil {
		_ = t.stdout.Close()
	}
	<-t.readerDone
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	_ = t.cmd.Wait()
	return nil
}

// Ensure StdioTransport implements Transport at compile time.
var _ Transport = (*StdioTransport)(nil)
