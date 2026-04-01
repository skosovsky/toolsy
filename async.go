package toolsy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
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

// asyncOptions holds configuration for AsAsyncTool.
type asyncOptions struct {
	onComplete AsyncCallback
}

// AsyncOption configures AsAsyncTool.
type AsyncOption func(*asyncOptions)

// WithOnComplete registers a hook called when the async task completes (with all collected chunks and final error).
func WithOnComplete(cb AsyncCallback) AsyncOption {
	return func(o *asyncOptions) {
		o.onComplete = cb
	}
}

// AsAsyncTool wraps a tool so that Execute returns immediately with AsyncAccepted;
// the base tool runs in a goroutine. If the client's yield returns an error (e.g. stream closed),
// the goroutine is not started (yield-guard).
// When executed via Registry, the registry injects an async tracker via RunContext; the background
// job is tracked so Shutdown waits for it to finish.
func AsAsyncTool(baseTool Tool, opts ...AsyncOption) Tool {
	var o asyncOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &asyncTool{
		toolBase: toolBase{next: baseTool},
		opts:     &o,
	}
}

type asyncTool struct {
	toolBase

	opts *asyncOptions
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
		r.registry.running.Done()
	}
}

func (t *asyncTool) Execute(ctx context.Context, run RunContext, input ToolInput, yield func(Chunk) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	taskID, err := generateTaskID()
	if err != nil {
		return err
	}
	accepted, err := json.Marshal(AsyncAccepted{Status: "accepted", TaskID: taskID})
	if err != nil {
		return &SystemError{Err: fmt.Errorf("async: marshal accepted payload: %w", err)}
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
	var bgDone func()
	if run.async != nil {
		bgDone = run.async.trackBackground()
	}
	go func(parentCtx context.Context) {
		if bgDone != nil {
			defer bgDone()
		}
		var collected []Chunk
		collectYield := func(c Chunk) error {
			collected = append(collected, c)
			return nil
		}

		baseCtx := context.WithoutCancel(parentCtx)
		bgRun := run
		bgRun.async = nil

		var executionErr error
		defer func() {
			if r := recover(); r != nil {
				executionErr = &SystemError{Err: &panicError{p: r}}
			}
			if t.opts.onComplete != nil {
				func() {
					defer func() { _ = recover() }() // isolate callback panic so it does not crash the process
					t.opts.onComplete(baseCtx, taskID, collected, executionErr)
				}()
			}
		}()

		executionErr = t.next.Execute(baseCtx, bgRun, input, collectYield)
	}(ctx)
	return nil
}

func generateTaskID() (string, error) {
	b := make([]byte, taskIDRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("async: generate task_id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
