package toolsy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// AsyncAccepted is the payload returned immediately when an async tool is invoked.
// The client (orchestrator) receives it via the first Chunk; the actual tool runs in a goroutine.
type AsyncAccepted struct {
	Status string `json:"status"`
	TaskID string `json:"task_id"`
}

// AsyncCallback is invoked when the wrapped tool finishes (success or error).
// chunks holds all chunks collected from the base tool's yield; err is the error returned by Execute.
type AsyncCallback func(ctx context.Context, taskID string, chunks []Chunk, err error)

// DefaultMaxCollectedChunks is the default cap on chunks buffered during async background execution.
const DefaultMaxCollectedChunks = 1000

// asyncOptions holds configuration for AsAsyncTool.
type asyncOptions struct {
	onComplete         AsyncCallback
	timeout            time.Duration
	maxCollectedChunks int
}

// AsyncOption configures AsAsyncTool.
type AsyncOption func(*asyncOptions)

// WithOnComplete registers a hook called when the async task completes (with all collected chunks and final error).
func WithOnComplete(cb AsyncCallback) AsyncOption {
	return func(o *asyncOptions) {
		o.onComplete = cb
	}
}

// WithBackgroundTimeout limits how long the background goroutine may run after the accepted
// response is returned. Parent cancellation is stripped via [context.WithoutCancel]; this option
// applies an independent deadline to background work only.
func WithBackgroundTimeout(d time.Duration) AsyncOption {
	return func(o *asyncOptions) {
		o.timeout = d
	}
}

// WithMaxCollectedChunks overrides the default cap on chunks buffered for [WithOnComplete].
// Values n <= 0 are ignored. Default is [DefaultMaxCollectedChunks] (1000).
// When the cap is exceeded, [ErrAsyncCollectedLimitExceeded] is surfaced in [WithOnComplete].
func WithMaxCollectedChunks(n int) AsyncOption {
	return func(o *asyncOptions) {
		if n > 0 {
			o.maxCollectedChunks = n
		}
	}
}

// AsAsyncTool wraps a standard Tool to execute its core logic in a background goroutine,
// immediately returning an AsyncAccepted chunk to the caller. If the client's yield returns
// an error (e.g. stream closed), the goroutine is not started (yield-guard).
//
// WARNING: Do not manually wrap the result of AsAsyncTool with middleware
// (e.g. WithBudget(AsAsyncTool(base))). Middleware applied that way runs only during the
// synchronous accept path and does not observe background work.
//
// CORRECT USAGE: register async tools via [RegistryBuilder]:
//
//	builder.Use(WithBudget).Add(AsAsyncTool(base)).Build()
//
// [RegistryBuilder.Build] unwraps the tool, applies Use() middleware inside the background
// goroutine, and re-wraps it safely. For direct Execute without a registry, wrap middleware
// around the base tool before AsAsyncTool. Nested AsAsyncTool(AsAsyncTool(...)) is invalid
// and rejected at [RegistryBuilder.Build] (including when manual middleware wraps nested
// async tools before Add). Manual middleware before Add must implement [ChainUnwrapper].
// Direct Execute without a registry does not re-check nesting.
//
// When executed via [Registry], the registry injects an async tracker via [*RunEnv]; the
// background job is tracked so [Registry.Shutdown] waits for it to finish.
func AsAsyncTool(baseTool Tool, opts ...AsyncOption) Tool {
	o := asyncOptions{ //nolint:exhaustruct // onComplete, timeout set via AsyncOption
		maxCollectedChunks: DefaultMaxCollectedChunks,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &asyncTool{
		toolBase: toolBase{next: baseTool},
		opts:     o,
	}
}

type asyncTool struct {
	toolBase

	opts asyncOptions
}

type asyncRuntime struct {
	registry          *Registry
	backgroundStarted atomic.Bool
}

func newAsyncRuntime(registry *Registry) *asyncRuntime {
	return &asyncRuntime{
		registry:          registry,
		backgroundStarted: atomic.Bool{},
	}
}

const taskIDRandomBytes = 16

// trackBackground marks the execution as having async follow-up work. The registry already called
// running.Add(1) in executeWithSummary; that single count is released here when the background
// goroutine finishes (sync paths release via defer in executeWithSummary instead).
func (r *asyncRuntime) trackBackground() func() {
	if r == nil || r.registry == nil {
		return nil
	}
	r.backgroundStarted.Store(true)
	return func() {
		r.registry.state.running.Done()
	}
}

func (t *asyncTool) Execute(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if env == nil {
		env = NewRunEnv(nil)
	}
	taskID, err := generateTaskID()
	if err != nil {
		return err
	}
	if err := t.yieldAsyncAccepted(ctx, taskID, yield); err != nil {
		return err
	}
	clonedInput := input.Clone()
	var bgDone func()
	if env.async != nil {
		bgDone = env.async.trackBackground()
	}
	go t.runBackground(ctx, env, clonedInput, taskID, bgDone)
	return nil
}

func (t *asyncTool) yieldAsyncAccepted(ctx context.Context, taskID string, yield func(Chunk) error) error {
	accepted, err := json.Marshal(AsyncAccepted{Status: "accepted", TaskID: taskID})
	if err != nil {
		return NewInternalError(fmt.Errorf("async: marshal accepted payload: %w", err))
	}
	chunk := Chunk{
		Event:    EventResult,
		Data:     accepted,
		MimeType: MimeTypeJSON,
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := validateChunk(chunk); err != nil {
		return err
	}
	if err := yield(chunk); err != nil {
		return wrapYieldError(err)
	}
	return nil
}

func (t *asyncTool) runBackground(
	parentCtx context.Context,
	env *RunEnv,
	input ToolInput,
	taskID string,
	bgDone func(),
) {
	if bgDone != nil {
		defer bgDone()
	}
	var (
		collected          []Chunk
		backgroundYieldErr error
	)
	collectYield := func(c Chunk) error {
		if len(collected) >= t.opts.maxCollectedChunks {
			err := fmt.Errorf("%w (%d)", ErrAsyncCollectedLimitExceeded, t.opts.maxCollectedChunks)
			if backgroundYieldErr == nil {
				backgroundYieldErr = err
			}
			return err
		}
		if err := validateChunk(c); err != nil {
			if backgroundYieldErr == nil {
				backgroundYieldErr = err
			}
			return err
		}
		collected = append(collected, c)
		return nil
	}

	baseCtx := context.WithoutCancel(parentCtx)
	if t.opts.timeout > 0 {
		var cancel context.CancelFunc
		baseCtx, cancel = context.WithTimeout(baseCtx, t.opts.timeout)
		defer cancel()
	}
	bgEnv := env.cloneForExecute(input.Attachments, env.async)

	var executionErr error
	defer func() {
		if r := recover(); r != nil {
			executionErr = NewInternalError(&panicError{p: r})
		}
		if t.opts.onComplete != nil {
			func() {
				defer func() { _ = recover() }() // isolate callback panic so it does not crash the process
				t.opts.onComplete(baseCtx, taskID, collected, executionErr)
			}()
		}
	}()

	executionErr = t.next.Execute(baseCtx, bgEnv, input, collectYield)
	if executionErr == nil {
		executionErr = backgroundYieldErr
	}
	if executionErr == nil {
		executionErr = executionErrFromCollected(collected)
	}
}

func generateTaskID() (string, error) {
	b := make([]byte, taskIDRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("async: generate task_id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func executionErrFromCollected(collected []Chunk) error {
	for _, c := range collected {
		if !c.IsError {
			continue
		}
		msg := strings.TrimSpace(string(c.Data))
		if msg == "" {
			return NewToolExecutionFailedError("background tool returned error chunk")
		}
		if !utf8.ValidString(msg) {
			msg = strings.ToValidUTF8(msg, "\uFFFD")
		}
		return NewToolExecutionFailedError(msg)
	}
	return nil
}
