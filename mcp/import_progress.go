package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
)

type progressCall struct {
	mu      sync.Mutex
	queue   chan toolsy.Chunk
	retired bool
	err     error
	cancel  context.CancelFunc
}

func (i *Importer) progress(params *sdk.ProgressNotificationParams) {
	if params == nil {
		return
	}
	token, ok := params.ProgressToken.(string)
	if !ok {
		return
	}
	i.mu.Lock()
	call := i.calls[token]
	i.mu.Unlock()
	if call == nil {
		return
	}
	call.mu.Lock()
	defer call.mu.Unlock()
	if call.retired {
		return
	}
	current := params.Progress
	total := params.Total
	chunk := toolsy.Chunk{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		Event: toolsy.EventProgress,
		Progress: &toolsy.ProgressInfo{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
			Current: &current,
			Total:   &total,
			Message: params.Message,
			Token:   token,
		},
	}
	select {
	case call.queue <- chunk:
	default:
		call.err = ErrQueueFull
		call.retired = true
		call.cancel()
	}
}

func (i *Importer) retire(value any) {
	token, ok := value.(string)
	if !ok {
		return
	}
	i.mu.Lock()
	call := i.calls[token]
	i.mu.Unlock()
	if call == nil {
		return
	}
	call.mu.Lock()
	call.retired = true
	call.mu.Unlock()
}

type callResult struct {
	result *sdk.CallToolResult
	err    error
}

func (i *Importer) call(
	ctx context.Context,
	generation uint64,
	name string,
	args []byte,
	hasSchema bool,
	yield func(toolsy.Chunk) error,
) error {
	release, err := i.admit()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := i.child(ctx)
	defer cancel()
	return i.callAdmitted(ctx, cancel, generation, name, args, hasSchema, yield)
}

func (i *Importer) admit() (func(), error) {
	select {
	case i.admission <- struct{}{}:
		return func() { <-i.admission }, nil
	default:
		return nil, &BridgeError{Kind: "admission", Cause: ErrAdmission}
	}
}

//nolint:gocognit // Centralized cancellation, progress draining and terminal ordering are one state machine.
func (i *Importer) callAdmitted(
	ctx context.Context,
	cancel context.CancelFunc,
	generation uint64,
	name string,
	args []byte,
	hasSchema bool,
	yield func(toolsy.Chunk) error,
) error {
	token := fmt.Sprintf("toolsy-%d", i.sequence.Add(1))
	state := &progressCall{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		queue:  make(chan toolsy.Chunk, i.config.ProgressBuffer),
		cancel: cancel,
	}
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return &BridgeError{Kind: "closed", Cause: ErrClosed}
	}
	i.calls[token] = state
	i.mu.Unlock()
	defer func() { i.retire(token); i.mu.Lock(); delete(i.calls, token); i.mu.Unlock() }()
	params := &sdk.CallToolParams{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		Name:      name,
		Arguments: json.RawMessage(bytes.Clone(args)),
	}
	params.SetProgressToken(token)
	if round, ok := ctx.Value(roundKey{}).(Round); ok {
		owned, err := cloneRound(round)
		if err != nil {
			return &BridgeError{Kind: "round", Cause: err}
		}
		params.RequestState, params.InputResponses = owned.RequestState, owned.InputResponses
	}
	terminal := make(chan callResult, 1)
	go func() {
		if staleErr := i.check(generation); staleErr != nil {
			terminal <- callResult{result: nil, err: staleErr}
			return
		}
		result, err := i.session.CallTool(ctx, params)
		// Install middleware retires before returning to this producer. Also
		// retire here defensively if a host middleware short-circuited the SDK.
		i.retire(token)
		terminal <- callResult{result: result, err: err}
	}()
	for {
		select {
		case <-ctx.Done():
			state.mu.Lock()
			overflow := state.err
			state.mu.Unlock()
			if overflow != nil {
				return &BridgeError{Kind: "progress overflow", Cause: overflow}
			}
			return &BridgeError{Kind: "cancelled", Cause: context.Cause(ctx)}
		case progress := <-state.queue:
			if err := yield(progress); err != nil {
				return err
			}
		case outcome := <-terminal:
			state.mu.Lock()
			overflow := state.err
			state.mu.Unlock()
			if overflow != nil {
				return &BridgeError{Kind: "progress overflow", Cause: overflow}
			}
			if ctx.Err() != nil {
				return &BridgeError{Kind: "cancelled", Cause: context.Cause(ctx)}
			}
			// Only this loop calls yield. Drain accepted progress before terminal.
			for {
				select {
				case progress := <-state.queue:
					if err := yield(progress); err != nil {
						return err
					}
				default:
					return finishImported(outcome, hasSchema, yield)
				}
			}
		}
	}
}

func finishImported(outcome callResult, hasSchema bool, yield func(toolsy.Chunk) error) error {
	if outcome.err != nil {
		return &BridgeError{Kind: "SDK call", Cause: outcome.err}
	}
	if outcome.result == nil {
		return &BridgeError{Kind: "nil result", Cause: ErrUnsupported}
	}
	result, err := cloneSDK(outcome.result)
	if err != nil {
		return &BridgeError{Kind: "result copy", Cause: err}
	}
	if result.NeedsInput() {
		return &NeedsInputError{Method: toolCallMethod, Result: result}
	}
	var raw []byte
	// v1.8.0 null and missing both become nil. Exact numbers have already been
	// rounded in StructuredContent/_meta; no adapter can recover their tokens.
	// https://github.com/modelcontextprotocol/go-sdk/pull/1239 (post-v1.8.0)
	if hasSchema && result.StructuredContent == nil && !result.IsError {
		return toolsy.NewInternalError(
			&toolsy.ResultContractError{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
				Kind: "missing_value",
			},
		)
	}
	if result.StructuredContent != nil {
		raw, err = json.Marshal(result.StructuredContent)
	} else {
		raw, err = json.Marshal(result)
	}
	if err != nil {
		return &BridgeError{Kind: "result projection", Cause: err}
	}
	chunk := toolsy.Chunk{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
		Event:       toolsy.EventResult,
		Data:        raw,
		MimeType:    toolsy.MimeTypeJSON,
		TypedResult: result,
		IsError:     result.IsError,
	}
	if result.IsError {
		failure := &toolsy.ToolError{ //nolint:exhaustruct_v5 // Optional payload fields and zero-value synchronization are deliberate.
			Code:      toolsy.CodeRemoteExecution,
			Reason:    "remote tool returned isError",
			Retryable: false,
		}
		chunk = toolsy.NewErrorChunkFromErr(failure)
		chunk.TypedResult = result
	}
	return yield(chunk)
}
