package toolsy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
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
// When executed via Registry, the registry injects an async tracker via context; the background
// job is then tracked so Shutdown waits for it and the concurrency slot is held until the job completes.
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

func (t *asyncTool) Execute(ctx context.Context, argsJSON []byte, yield func(Chunk) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	taskID, err := generateTaskID()
	if err != nil {
		return err
	}
	chunk := Chunk{
		Event:   EventResult,
		RawData: AsyncAccepted{Status: "accepted", TaskID: taskID},
	}
	// Re-check ctx immediately before yield so "cancelled before accepted => no accepted, no background".
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := yield(chunk); err != nil {
		return wrapYieldError(err)
	}
	var bgDone func()
	var bgTimeout time.Duration
	if v := ctx.Value(ctxKeyAsyncTrackerVal); v != nil {
		if tr, ok := v.(*asyncTracker); ok {
			if tr.track != nil {
				bgDone = tr.track()
			}
			if tr.effectiveTimeout > 0 {
				bgTimeout = tr.effectiveTimeout
			}
		}
	}
	if bgTimeout == 0 {
		bgTimeout = t.Timeout()
	}
	go func() {
		if bgDone != nil {
			defer bgDone()
		}
		var collected []Chunk
		collectYield := func(c Chunk) error {
			collected = append(collected, c)
			return nil
		}
		bgCtx := context.Background()
		if bgTimeout > 0 {
			var cancel context.CancelFunc
			bgCtx, cancel = context.WithTimeout(bgCtx, bgTimeout)
			defer cancel()
		}
		var executionErr error
		defer func() {
			if r := recover(); r != nil {
				executionErr = &SystemError{Err: &panicError{p: r}}
			}
			if t.opts.onComplete != nil {
				func() {
					defer func() { _ = recover() }() // isolate callback panic so it does not crash the process
					t.opts.onComplete(context.Background(), taskID, collected, executionErr)
				}()
			}
		}()
		executionErr = t.next.Execute(bgCtx, argsJSON, collectYield)
	}()
	return nil
}

func generateTaskID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("async: generate task_id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
