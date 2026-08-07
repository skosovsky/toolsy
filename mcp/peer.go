//nolint:exhaustruct // Internal JSON-RPC envelopes intentionally omit mutually exclusive fields.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

type callResult struct {
	result json.RawMessage
	err    error
}

const (
	maxCancelledResponseRanges = 1024
	decimalBase                = 10
	maxRPCIDBytes              = 1024
)

type rpcIDRange struct {
	first uint64
	last  uint64
}

type pendingRequest struct {
	id           json.RawMessage
	key          string
	peer         *rpcPeer
	ch           chan callResult
	once         sync.Once
	terminalOnce sync.Once
	terminal     chan struct{}
	deliveryOnce sync.Once
	deliveryDone chan struct{}
	sent         atomic.Bool
	cancelled    atomic.Bool
	cancelNotify atomic.Bool
	mu           sync.Mutex
	done         bool
	hooks        []func()
}

func (p *pendingRequest) ID() json.RawMessage { return bytes.Clone(p.id) }

func (p *pendingRequest) DeliveryDone() <-chan struct{} { return p.deliveryDone }
func (p *pendingRequest) WasSent() bool                 { return p.sent.Load() }

func (p *pendingRequest) CancelPending() bool {
	if p.cancelled.Load() {
		return true
	}
	return p.peer.cancelPending(p)
}

func (p *pendingRequest) ClaimCancellationNotification() bool {
	return p.cancelNotify.CompareAndSwap(false, true)
}

func (p *pendingRequest) markSent() {
	p.sent.Store(true)
	p.finishDelivery()
}

func (p *pendingRequest) finishDelivery() {
	p.deliveryOnce.Do(func() { close(p.deliveryDone) })
}

func (p *pendingRequest) Await(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		if p.CancelPending() {
			return nil, ctx.Err()
		}
		result, ok := <-p.ch
		if !ok {
			return nil, ErrTransportClosed
		}
		return bytes.Clone(result.result), result.err
	case result, ok := <-p.ch:
		if !ok {
			return nil, ErrTransportClosed
		}
		return bytes.Clone(result.result), result.err
	}
}

func (p *pendingRequest) complete(result callResult) {
	p.once.Do(func() {
		p.signalTerminal()
		p.mu.Lock()
		p.done = true
		hooks := append([]func(){}, p.hooks...)
		p.hooks = nil
		p.mu.Unlock()
		for _, hook := range hooks {
			hook()
		}
		p.ch <- result
		close(p.ch)
	})
}

func (p *pendingRequest) signalTerminal() {
	p.terminalOnce.Do(func() { close(p.terminal) })
}

func (p *pendingRequest) OnComplete(hook func()) {
	if hook == nil {
		return
	}
	p.mu.Lock()
	if !p.done {
		p.hooks = append(p.hooks, hook)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	hook()
}

type rpcPeer struct {
	logger *slog.Logger
	send   func(context.Context, []byte) error
	ctx    context.Context

	requestID       atomic.Uint64
	pendingMu       sync.Mutex
	pending         map[string]*pendingRequest
	cancelledRanges []rpcIDRange
	notifyMu        sync.RWMutex
	notify          map[string]NotificationHandler
	closeOnce       sync.Once
	closed          atomic.Bool
	cancel          context.CancelFunc
}

func newRPCPeer(
	ctx context.Context,
	logger *slog.Logger,
	send func(context.Context, []byte) error,
) *rpcPeer {
	if logger == nil {
		logger = slog.Default()
	}
	//nolint:gosec // The peer owns cancel and invokes it exactly once from rpcPeer.close.
	peerCtx, cancel := context.WithCancel(ctx)
	return &rpcPeer{
		logger:  logger,
		send:    send,
		ctx:     peerCtx,
		pending: make(map[string]*pendingRequest),
		notify:  make(map[string]NotificationHandler),
		cancel:  cancel,
	}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (p *rpcPeer) beginRequest(method string, params any) (*pendingRequest, []byte, error) {
	if p.closed.Load() {
		return nil, nil, ErrTransportClosed
	}
	paramsRaw, err := marshalParams(params)
	if err != nil {
		return nil, nil, err
	}
	id := p.requestID.Add(1)
	idRaw := json.RawMessage(strconv.FormatUint(id, 10))
	key, err := rpcIDKey(idRaw)
	if err != nil {
		return nil, nil, err
	}
	req := Request{JSONRPC: JSONRPCVersion, ID: idRaw, Method: method, Params: paramsRaw}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	pending := &pendingRequest{
		id:           bytes.Clone(idRaw),
		key:          key,
		peer:         p,
		ch:           make(chan callResult, 1),
		terminal:     make(chan struct{}),
		deliveryDone: make(chan struct{}),
	}
	p.pendingMu.Lock()
	if p.closed.Load() {
		p.pendingMu.Unlock()
		return nil, nil, ErrTransportClosed
	}
	p.pending[key] = pending
	p.pendingMu.Unlock()
	return pending, body, nil
}

func (p *rpcPeer) cancelPending(expected *pendingRequest) bool {
	removed := false
	var overflow error
	p.pendingMu.Lock()
	if p.pending[expected.key] == expected {
		expected.cancelled.Store(true)
		delete(p.pending, expected.key)
		id, idErr := strconv.ParseUint(string(expected.id), 10, 64)
		if idErr != nil || !p.addCancelledLocked(id) {
			overflow = &InvalidPayloadError{
				Subject: "JSON-RPC cancellation correlation",
				Err:     errors.New("cancelled response range capacity exhausted"),
			}
		}
		removed = true
	}
	p.pendingMu.Unlock()
	if removed {
		expected.complete(callResult{err: context.Canceled})
	}
	if overflow != nil {
		p.close(overflow)
	}
	return removed || expected.cancelled.Load()
}

func (p *rpcPeer) failPending(pending *pendingRequest, err error) {
	var stateErr error
	p.pendingMu.Lock()
	claimed := p.pending[pending.key] == pending
	if claimed {
		delete(p.pending, pending.key)
	} else if pending.cancelled.Load() && !pending.sent.Load() {
		_, stateErr = p.consumeCancelledLocked(pending.key)
	}
	p.pendingMu.Unlock()
	if stateErr != nil {
		p.close(stateErr)
	}
	if claimed {
		pending.complete(callResult{err: err})
	}
}

func (p *rpcPeer) notifyMessage(ctx context.Context, method string, params any) error {
	if p.closed.Load() {
		return ErrTransportClosed
	}
	paramsRaw, err := marshalParams(params)
	if err != nil {
		return err
	}
	body, err := json.Marshal(
		Notification{JSONRPC: JSONRPCVersion, Method: method, Params: paramsRaw},
	)
	if err != nil {
		return err
	}
	if p.closed.Load() {
		return ErrTransportClosed
	}
	return p.send(ctx, body)
}

func (p *rpcPeer) setNotificationHandler(method string, handler NotificationHandler) {
	p.notifyMu.Lock()
	if handler == nil {
		delete(p.notify, method)
	} else {
		p.notify[method] = handler
	}
	p.notifyMu.Unlock()
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *JSONRPCError   `json:"error"`
}

func (p *rpcPeer) dispatch(data []byte) error {
	return p.dispatchWithNotificationHandler(data, nil)
}

func (p *rpcPeer) dispatchWithNotificationHandler(data []byte, override NotificationHandler) error {
	const jsonRPCSubject = "JSON-RPC"
	if p.closed.Load() {
		return ErrTransportClosed
	}
	fields, err := p.decodeRPCObject(data)
	if err != nil {
		return err
	}

	var message rpcEnvelope
	if err := json.Unmarshal(data, &message); err != nil {
		payloadErr := &InvalidPayloadError{Subject: jsonRPCSubject, Err: err}
		if _, requestShaped := fields["method"]; !requestShaped {
			p.failCorrelatedResponse(fields["id"], payloadErr)
		}
		return payloadErr
	}
	_, hasMethod := fields["method"]
	_, hasResult := fields["result"]
	_, hasError := fields["error"]
	_, hasParams := fields["params"]
	if message.JSONRPC != JSONRPCVersion {
		payloadErr := &InvalidPayloadError{
			Subject: jsonRPCSubject,
			Err:     fmt.Errorf("invalid jsonrpc version %q", message.JSONRPC),
		}
		if !hasMethod {
			p.failCorrelatedResponse(fields["id"], payloadErr)
		}
		return payloadErr
	}
	return p.dispatchEnvelope(fields, message, hasMethod, hasResult, hasError, hasParams, override)
}

func (p *rpcPeer) decodeRPCObject(data []byte) (map[string]json.RawMessage, error) {
	const jsonRPCSubject = "JSON-RPC"
	if !utf8.Valid(data) {
		return nil, &InvalidPayloadError{Subject: jsonRPCSubject, Err: errors.New("payload is not valid UTF-8")}
	}

	if err := validateJSONValue(data); err != nil {
		return nil, &InvalidPayloadError{Subject: jsonRPCSubject, Err: err}
	}
	fields, err := decodeObjectFields(data)
	if err != nil {
		return nil, &InvalidPayloadError{Subject: jsonRPCSubject, Err: err}
	}
	return fields, nil
}

//nolint:gocognit,nestif // Request/response union validation is intentionally centralized.
func (p *rpcPeer) dispatchEnvelope(
	fields map[string]json.RawMessage,
	message rpcEnvelope,
	hasMethod bool,
	hasResult bool,
	hasError bool,
	hasParams bool,
	notificationOverride NotificationHandler,
) error {
	if hasMethod {
		paramsInvalid := false
		methodInvalid := requireStringField(fields, "method", true) != nil
		if hasParams {
			_, paramsErr := decodeObjectFields(message.Params)
			paramsInvalid = paramsErr != nil
		}
		if methodInvalid || hasResult || hasError || paramsInvalid {
			return &InvalidPayloadError{
				Subject: "JSON-RPC request",
				Err:     errors.New("request must contain method, object params, and no result/error fields"),
			}
		}
		if len(message.ID) == 0 {
			if isForbiddenLegacyMethod(message.Method) {
				return &InvalidPayloadError{
					Subject: "JSON-RPC notification",
					Err:     fmt.Errorf("legacy method %q is forbidden", message.Method),
				}
			}
			handler := notificationOverride
			if handler == nil {
				p.notifyMu.RLock()
				handler = p.notify[message.Method]
				p.notifyMu.RUnlock()
			}
			if handler != nil {
				handler(bytes.Clone(message.Params))
			}
			return nil
		}
		return &InvalidPayloadError{
			Subject: "JSON-RPC request",
			Err:     errors.New("server-initiated requests are not supported by MCP 2026-07-28"),
		}
	}
	if hasParams || len(message.ID) == 0 || hasResult == hasError || (hasError && message.Error == nil) {
		payloadErr := &InvalidPayloadError{
			Subject: "JSON-RPC response",
			Err:     errors.New("response must contain id and exactly one of result or error"),
		}
		// A malformed response with a valid, active correlation ID must still
		// complete that call. Stdio deliberately keeps reading after bad frames,
		// so merely returning the validation error would otherwise hang Await.
		p.failCorrelatedResponse(message.ID, payloadErr)
		return payloadErr
	}
	key, err := rpcIDKey(message.ID)
	if err != nil {
		return &InvalidPayloadError{Subject: "JSON-RPC response id", Err: err}
	}
	p.pendingMu.Lock()
	pending := p.pending[key]
	if pending != nil {
		delete(p.pending, key)
	} else if cancelled, stateErr := p.consumeCancelledLocked(key); stateErr != nil {
		p.pendingMu.Unlock()
		p.close(stateErr)
		return stateErr
	} else if cancelled {
		p.pendingMu.Unlock()
		return nil
	}
	p.pendingMu.Unlock()
	if pending == nil {
		return &InvalidPayloadError{
			Subject: "JSON-RPC response",
			Err:     errors.New("unknown or duplicate response id"),
		}
	}
	if hasError {
		if validationErr := validateJSONRPCError(fields["error"]); validationErr != nil {
			payloadErr := &InvalidPayloadError{Subject: "JSON-RPC response error", Err: validationErr}
			pending.complete(callResult{err: payloadErr})
			return payloadErr
		}
	}
	if message.Error != nil {
		rpcErr := &RPCError{
			Code:    message.Error.Code,
			Message: message.Error.Message,
			Data:    bytes.Clone(message.Error.Data),
		}
		pending.complete(
			callResult{
				err: TypedRPCError(rpcErr),
			},
		)
	} else {
		pending.complete(callResult{result: bytes.Clone(message.Result)})
	}
	return nil
}

func (p *rpcPeer) addCancelledLocked(id uint64) bool {
	if len(p.cancelledRanges) == 0 {
		p.cancelledRanges = append(p.cancelledRanges, rpcIDRange{first: id, last: id})
		return true
	}
	for index := range p.cancelledRanges {
		current := &p.cancelledRanges[index]
		switch {
		case id >= current.first && id <= current.last:
			return true
		case id != ^uint64(0) && id+1 == current.first:
			current.first = id
			if index > 0 && p.cancelledRanges[index-1].last != ^uint64(0) &&
				p.cancelledRanges[index-1].last+1 == current.first {
				p.cancelledRanges[index-1].last = current.last
				p.cancelledRanges = append(
					p.cancelledRanges[:index],
					p.cancelledRanges[index+1:]...,
				)
			}
			return true
		case current.last != ^uint64(0) && id == current.last+1:
			current.last = id
			if index+1 < len(p.cancelledRanges) && id != ^uint64(0) &&
				id+1 == p.cancelledRanges[index+1].first {
				current.last = p.cancelledRanges[index+1].last
				p.cancelledRanges = append(
					p.cancelledRanges[:index+1],
					p.cancelledRanges[index+2:]...,
				)
			}
			return true
		case id < current.first:
			if len(p.cancelledRanges) == maxCancelledResponseRanges {
				return false
			}
			p.cancelledRanges = append(p.cancelledRanges, rpcIDRange{})
			copy(p.cancelledRanges[index+1:], p.cancelledRanges[index:])
			p.cancelledRanges[index] = rpcIDRange{first: id, last: id}
			return true
		}
	}
	if len(p.cancelledRanges) == maxCancelledResponseRanges {
		return false
	}
	p.cancelledRanges = append(p.cancelledRanges, rpcIDRange{first: id, last: id})
	return true
}

func (p *rpcPeer) consumeCancelledLocked(key string) (bool, error) {
	id, ok := cancelledNumericID(key)
	if !ok {
		return false, nil
	}
	removed, overflow := p.removeCancelledIDLocked(id)
	if overflow {
		return false, &InvalidPayloadError{
			Subject: "JSON-RPC cancellation correlation",
			Err:     errors.New("cancelled response range fragmentation exhausted"),
		}
	}
	return removed, nil
}

func cancelledNumericID(key string) (uint64, bool) {
	if len(key) < 3 || key[:2] != "n:" {
		return 0, false
	}
	coefficient, exponentText, ok := strings.Cut(key[2:], "e")
	if !ok {
		id, err := strconv.ParseUint(key[2:], decimalBase, 64)
		return id, err == nil
	}
	if strings.HasPrefix(coefficient, "-") {
		return 0, false
	}
	var exponent big.Int
	if _, ok := exponent.SetString(exponentText, decimalBase); !ok || !exponent.IsInt64() {
		return 0, false
	}
	zeroCount := exponent.Int64()
	if zeroCount < 0 || int64(len(coefficient))+zeroCount > 20 {
		return 0, false
	}
	decimal := coefficient + strings.Repeat("0", int(zeroCount))
	id, err := strconv.ParseUint(decimal, 10, 64)
	return id, err == nil
}

func (p *rpcPeer) removeCancelledIDLocked(id uint64) (bool, bool) {
	for index, current := range p.cancelledRanges {
		if id < current.first {
			return false, false
		}
		if id > current.last {
			continue
		}
		switch {
		case current.first == current.last:
			p.cancelledRanges = append(
				p.cancelledRanges[:index],
				p.cancelledRanges[index+1:]...,
			)
		case id == current.first:
			p.cancelledRanges[index].first++
		case id == current.last:
			p.cancelledRanges[index].last--
		default:
			if len(p.cancelledRanges) == maxCancelledResponseRanges {
				return false, true
			}
			right := rpcIDRange{first: id + 1, last: current.last}
			p.cancelledRanges[index].last = id - 1
			p.cancelledRanges = append(p.cancelledRanges, rpcIDRange{})
			copy(p.cancelledRanges[index+2:], p.cancelledRanges[index+1:])
			p.cancelledRanges[index+1] = right
		}
		return true, false
	}
	return false, false
}

func (p *rpcPeer) failCorrelatedResponse(id json.RawMessage, err error) {
	key, keyErr := rpcIDKey(id)
	if keyErr != nil {
		return
	}
	p.pendingMu.Lock()
	pending := p.pending[key]
	if pending != nil {
		delete(p.pending, key)
	}
	p.pendingMu.Unlock()
	if pending != nil {
		pending.complete(callResult{err: err})
	}
}

func validateJSONRPCError(raw json.RawMessage) error {
	fields, err := decodeObjectFields(raw)
	if err != nil {
		return err
	}
	code, ok := fields["code"]
	if !ok {
		return errors.New("error.code is required")
	}
	if bytes.Equal(bytes.TrimSpace(code), []byte(jsonNull)) {
		return errors.New("error.code must be a non-null number")
	}
	var number JSONNumber
	if err := json.Unmarshal(code, &number); err != nil {
		return fmt.Errorf("error.code must be a number: %w", err)
	}
	if !isIntegralJSONNumber(number) {
		return errors.New("error.code must be an integer")
	}
	if err := requireStringField(fields, "message", true); err != nil {
		return fmt.Errorf("invalid error.message: %w", err)
	}
	return nil
}

func (p *rpcPeer) close(err error) {
	if err == nil {
		err = ErrTransportClosed
	}
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		p.pendingMu.Lock()
		pending := p.pending
		p.pending = make(map[string]*pendingRequest)
		p.cancelledRanges = nil
		p.pendingMu.Unlock()
		for _, request := range pending {
			request.complete(callResult{err: err})
		}
	})
}

func rpcIDKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || len(raw) > maxRPCIDBytes {
		return "", errors.New("id exceeds the maximum encoded size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	switch id := value.(type) {
	case string:
		return "s:" + id, nil
	case json.Number:
		if !isIntegralJSONNumber(JSONNumber(id.String())) {
			return "", errors.New("id must be a string or integer")
		}
		canonical, ok := canonicalJSONNumber(id.String())
		if !ok {
			return "", errors.New("id must be a string or integer")
		}
		return "n:" + canonical, nil
	default:
		return "", errors.New("id must be a string or integer")
	}
}

// canonicalJSONNumber returns a compact exact decimal scientific form. It
// canonicalizes equivalent JSON numbers without expanding exponent magnitude.
func canonicalJSONNumber(value string) (string, bool) {
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	mantissa, exponentText, hasExponent := strings.Cut(value, "e")
	if !hasExponent {
		mantissa, exponentText, hasExponent = strings.Cut(value, "E")
	}
	var exponent big.Int
	if hasExponent {
		if _, ok := exponent.SetString(exponentText, decimalBase); !ok {
			return "", false
		}
	}
	fractionalDigits := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fractionalDigits = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", true
	}
	trailingZeros := len(digits) - len(strings.TrimRight(digits, "0"))
	digits = digits[:len(digits)-trailingZeros]
	exponent.Sub(&exponent, big.NewInt(int64(fractionalDigits)))
	exponent.Add(&exponent, big.NewInt(int64(trailingZeros)))
	if negative {
		digits = "-" + digits
	}
	if exponent.Sign() == 0 {
		return digits, true
	}
	return digits + "e" + exponent.String(), true
}

func isIntegralJSONNumber(value JSONNumber) bool {
	if !jsonNumberPattern.MatchString(value.String()) {
		return false
	}
	canonical, ok := canonicalJSONNumber(value.String())
	if !ok {
		return false
	}
	_, exponent, hasExponent := strings.Cut(canonical, "e")
	return !hasExponent || !strings.HasPrefix(exponent, "-")
}
