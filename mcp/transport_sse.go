package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	rpcJSONLineScannerMaxBytes = 1024 * 1024
	sseCallResponseTimeout     = 30 * time.Second
)

// SSETransport connects to an MCP server over HTTP SSE. The initial URL is used for GET (event stream);
// the POST endpoint is received dynamically in the first SSE event of type "endpoint" (MCP spec).
type SSETransport struct {
	impl *sseTransportImpl
}

type sseTransportImpl struct {
	initialURL string
	client     *http.Client

	startMu   sync.Mutex
	started   bool
	startErr  error
	postURL   string
	streamErr error
	postURLMu sync.RWMutex
	ready     chan struct{} // closed when endpoint received or stream ended
	readyOnce sync.Once

	requestID atomic.Uint64
	pending   sync.Map // id string -> chan *sseCallResult

	readerDone chan struct{}
	bodyCloser io.Closer

	notifyMu       sync.Mutex
	notifyHandlers map[string]func(params []byte)
}

// callResult holds result or error for a pending Call (shared with stdio).
type sseCallResult struct {
	Result []byte
	Err    error
}

// NewSSETransport creates an SSE transport. initialURL is the URL for the GET request (e.g. http://localhost:3001/sse).
// The server must send an event with type "endpoint" first; the "data" field contains the URL for POST (Call/Notify).
func NewSSETransport(initialURL string) *SSETransport {
	impl := &sseTransportImpl{
		initialURL: initialURL,
		// Timeout 0: no per-request cap on the client; long-lived GET stream and
		// per-call timeouts are handled at the transport layer (e.g. sseCallResponseTimeout).
		client:         &http.Client{Timeout: 0},
		startMu:        sync.Mutex{},
		started:        false,
		startErr:       nil,
		postURL:        "",
		streamErr:      nil,
		postURLMu:      sync.RWMutex{},
		ready:          make(chan struct{}),
		readyOnce:      sync.Once{},
		requestID:      atomic.Uint64{},
		pending:        sync.Map{},
		readerDone:     make(chan struct{}),
		bodyCloser:     nil,
		notifyMu:       sync.Mutex{},
		notifyHandlers: make(map[string]func(params []byte)),
	}
	return &SSETransport{impl: impl}
}

// Start starts the GET request to initialURL and begins reading the SSE stream.
// Blocks until the first "endpoint" event is received (or context is done).
func (s *SSETransport) Start(ctx context.Context) error {
	return s.impl.start(ctx)
}

func (t *sseTransportImpl) start(ctx context.Context) error {
	t.startMu.Lock()
	if t.started {
		t.startMu.Unlock()
		if t.startErr != nil {
			return t.startErr
		}
		<-t.ready
		return nil
	}
	t.started = true
	t.startMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.initialURL, nil)
	if err != nil {
		t.startErr = err
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	// #nosec G704 -- URL is from user config (initialURL) or server endpoint event; caller is responsible for trust.
	resp, err := t.client.Do(req)
	if err != nil {
		t.startErr = err
		return err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.startErr = fmt.Errorf("sse GET %s: status %d", t.initialURL, resp.StatusCode)
		return t.startErr
	}
	t.bodyCloser = resp.Body

	go t.readLoop(resp.Body)
	select {
	case <-ctx.Done():
		_ = resp.Body.Close()
		t.startErr = ctx.Err()
		return ctx.Err()
	case <-t.ready:
		return nil
	}
}

// readLoop parses SSE events. First event must be "endpoint" with data = POST URL.
// Subsequent events are JSON-RPC messages (response or notification).
func (t *sseTransportImpl) readLoop(body io.Reader) {
	defer close(t.readerDone)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(nil, rpcJSONLineScannerMaxBytes)
	var eventType, data string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventType != "" {
				t.handleSSEEvent(eventType, data)
			}
			eventType = ""
			data = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[6:])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = strings.TrimSpace(line[5:])
		}
	}
	err := scanner.Err()
	if err == nil {
		err = errors.New("sse stream closed")
	}
	t.unblockAllPending(err)
	t.postURLMu.Lock()
	if t.postURL == "" {
		t.streamErr = err
	}
	t.postURLMu.Unlock()
	t.readyOnce.Do(func() { close(t.ready) })
}

func (t *sseTransportImpl) handleSSEEvent(eventType, data string) {
	if eventType == "endpoint" {
		postURL := strings.TrimSpace(data)
		if postURL != "" {
			t.postURLMu.Lock()
			t.postURL = postURL
			t.postURLMu.Unlock()
			t.readyOnce.Do(func() { close(t.ready) })
		}
		return
	}
	if data == "" {
		return
	}
	t.dispatchSSEJSONRPCData(data)
}

func (t *sseTransportImpl) dispatchSSEJSONRPCData(data string) {
	var raw struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
		Error  *JSONRPCError   `json:"error"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return
	}
	if raw.Method != "" && raw.Result == nil && raw.Error == nil {
		t.notifyMu.Lock()
		handler := t.notifyHandlers[raw.Method]
		t.notifyMu.Unlock()
		if handler != nil {
			handler(raw.Params)
		}
		return
	}
	if len(raw.ID) == 0 || (raw.Result == nil && raw.Error == nil) {
		return
	}
	var idAny any
	if json.Unmarshal(raw.ID, &idAny) != nil {
		return
	}
	id := fmt.Sprint(idAny)
	if chVal, ok := t.pending.LoadAndDelete(id); ok {
		ch, chOK := chVal.(chan *sseCallResult)
		if !chOK {
			return
		}
		res := &sseCallResult{Result: nil, Err: nil}
		if raw.Error != nil {
			res.Err = fmt.Errorf("json-rpc error %d: %s", raw.Error.Code, raw.Error.Message)
		} else {
			res.Result = raw.Result
		}
		select {
		case ch <- res:
		default:
		}
		close(ch)
	}
}

func (t *sseTransportImpl) unblockAllPending(err error) {
	t.pending.Range(func(key, value any) bool {
		t.pending.Delete(key)
		ch, ok := value.(chan *sseCallResult)
		if !ok {
			return true
		}
		select {
		case ch <- &sseCallResult{Result: nil, Err: err}:
		default:
		}
		close(ch)
		return true
	})
}

func (t *sseTransportImpl) getPostURL() (string, error) {
	select {
	case <-t.ready:
	default:
		return "", errors.New("endpoint not yet received")
	}
	t.postURLMu.RLock()
	u := t.postURL
	err := t.streamErr
	t.postURLMu.RUnlock()
	if u == "" && err != nil {
		return "", err
	}
	if u == "" {
		return "", errors.New("endpoint not received")
	}
	return u, nil
}

// Call sends a JSON-RPC request via POST to the endpoint URL and waits for the response in the SSE stream.
func (s *SSETransport) Call(ctx context.Context, method string, params any) ([]byte, string, error) {
	return s.impl.call(ctx, method, params)
}

func (t *sseTransportImpl) call(ctx context.Context, method string, params any) ([]byte, string, error) {
	postURL, err := t.getPostURL()
	if err != nil {
		return nil, "", err
	}
	var paramsRaw json.RawMessage
	if params != nil {
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return nil, "", err
		}
	}
	id := t.requestID.Add(1)
	idStr := strconv.FormatUint(id, 10)
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

	ch := make(chan *sseCallResult, 1)
	t.pending.Store(idStr, ch)
	defer t.pending.Delete(idStr)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Resolve relative POST URL against initial URL base.
	base, err := url.Parse(t.initialURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid initial URL: %w", err)
	}
	if postU, parseErr := base.Parse(postURL); parseErr == nil {
		httpReq.URL = postU
	}

	// #nosec G704 -- POST URL is from user config or server endpoint event; caller is responsible for trust.
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	_ = resp.Body.Close()

	timer := time.NewTimer(sseCallResponseTimeout)
	defer timer.Stop()
	// Response arrives asynchronously via the SSE stream; wait for it.
	select {
	case <-ctx.Done():
		return nil, idStr, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, idStr, res.Err
		}
		return res.Result, idStr, nil
	case <-timer.C:
		return nil, idStr, fmt.Errorf("timeout waiting for response to %s", method)
	}
}

// Notify sends a JSON-RPC notification via POST.
func (s *SSETransport) Notify(ctx context.Context, method string, params any) error {
	return s.impl.notify(ctx, method, params)
}

func (t *sseTransportImpl) notify(ctx context.Context, method string, params any) error {
	postURL, err := t.getPostURL()
	if err != nil {
		return err
	}
	var paramsRaw json.RawMessage
	if params != nil {
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	base, err := url.Parse(t.initialURL)
	if err != nil {
		return fmt.Errorf("invalid initial URL: %w", err)
	}
	postU, _ := base.Parse(postURL)
	if postU != nil {
		httpReq.URL = postU
	}
	// #nosec G704 -- POST URL is from user config or server endpoint event; caller is responsible for trust.
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// OnNotification registers a handler for incoming notifications.
func (s *SSETransport) OnNotification(method string, handler func(params []byte)) {
	s.impl.notifyMu.Lock()
	defer s.impl.notifyMu.Unlock()
	s.impl.notifyHandlers[method] = handler
}

// Close closes the SSE connection and unblocks pending Call.
func (s *SSETransport) Close() error {
	return s.impl.close()
}

func (t *sseTransportImpl) close() error {
	t.startMu.Lock()
	if !t.started {
		t.startMu.Unlock()
		return nil
	}
	t.started = false
	t.startMu.Unlock()
	if t.bodyCloser != nil {
		_ = t.bodyCloser.Close()
	}
	<-t.readerDone
	t.unblockAllPending(errors.New("transport closed"))
	return nil
}

var _ Transport = (*SSETransport)(nil)
