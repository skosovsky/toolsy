package toolsy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Registry holds tools and executes them with timeout, semaphore, and optional panic recovery.
type Registry struct {
	tools       map[string]Tool // wrapped with middlewares, used by Execute
	rawTools    map[string]Tool // unwrapped, used by Use() to re-apply middlewares from scratch
	sem         chan struct{}
	opts        registryOptions
	done        chan struct{}
	running     sync.WaitGroup
	mu          sync.Mutex
	middlewares []Middleware
}

// NewRegistry creates a Registry with the given options.
func NewRegistry(opts ...RegistryOption) *Registry {
	o := registryOptions{
		timeout:        5 * time.Second,
		maxConcurrency: 10,
		recoverPanics:  true,
	}
	for _, opt := range opts {
		opt(&o)
	}
	var sem chan struct{}
	if o.maxConcurrency > 0 {
		sem = make(chan struct{}, o.maxConcurrency)
	}
	return &Registry{
		tools:    make(map[string]Tool),
		rawTools: make(map[string]Tool),
		sem:      sem,
		opts:     o,
		done:     make(chan struct{}),
	}
}

// Register adds a tool. Stored middlewares (see Use) are applied to the tool before registration.
// If a tool with the same name already exists, it is replaced. Safe for concurrent use with Execute and other Register calls.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	r.rawTools[name] = t
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		t = r.middlewares[i](t)
	}
	r.tools[name] = t
}

// GetAllTools returns all registered tools (e.g. for exporting to LLM providers), sorted by name for deterministic order.
func (r *Registry) GetAllTools() []Tool {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

// GetTool returns the tool with the given name (after middlewares are applied), or (nil, false) if not found.
func (r *Registry) GetTool(name string) (Tool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tools[name]
	return t, ok
}

// Execute runs one tool call and streams chunks to yield. Returns on first yield error or tool error.
// The after-execution hook (WithOnAfterExecute) is always invoked via defer with ExecutionSummary.
func (r *Registry) Execute(ctx context.Context, call ToolCall, yield func([]byte) error) (err error) {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return ErrShutdown
	default:
	}
	tool, ok := r.tools[call.ToolName]
	if !ok {
		r.mu.Unlock()
		return ErrToolNotFound
	}
	r.running.Add(1)
	r.mu.Unlock()

	if err = r.acquireSemaphore(ctx); err != nil {
		r.running.Done()
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrTimeout
		}
		return err
	}
	defer r.releaseSemaphore()
	defer r.running.Done()

	timeout := r.opts.timeout
	if tm, ok := tool.(ToolMetadata); ok && tm.Timeout() > 0 {
		timeout = tm.Timeout()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var summary ExecutionSummary
	summary.CallID = call.ID
	summary.ToolName = call.ToolName
	start := time.Now()
	// Ensure after-execution hook is always called with final summary (partial success or error).
	// Recover defer is registered after onAfter so it runs first on panic and sets summary.Error before the hook runs.
	defer func() {
		dur := time.Since(start)
		if r.opts.onAfter != nil {
			r.opts.onAfter(ctx, call, summary, dur)
		}
	}()
	if r.opts.recoverPanics {
		defer func() {
			if p := recover(); p != nil {
				summary.Error = &SystemError{Err: &panicError{p: p}}
				err = summary.Error
			}
		}()
	}

	if r.opts.onBefore != nil {
		r.opts.onBefore(ctx, call)
	}

	// Wrap yield to count chunks/bytes and optionally call onChunk. onChunk is only invoked for successfully delivered chunks.
	yieldWrapped := func(chunk []byte) error {
		err := yield(chunk)
		if err == nil {
			summary.ChunksDelivered++
			summary.TotalBytes += int64(len(chunk))
			if r.opts.onChunk != nil {
				r.opts.onChunk(ctx, Chunk{CallID: call.ID, ToolName: call.ToolName, Data: chunk})
			}
		}
		return err
	}

	summary.Error = tool.Execute(ctx, call.Args, yieldWrapped)
	return summary.Error
}

func (r *Registry) acquireSemaphore(ctx context.Context) error {
	if r.sem == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case r.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Registry) releaseSemaphore() {
	if r.sem != nil {
		<-r.sem
	}
}

// ExecuteBatchStream runs all calls in parallel and streams chunks via yield. Each chunk is
// tagged with CallID and ToolName (Chunk). The library serializes calls to yield with a mutex
// so the caller's yield does not need to be thread-safe. Returns on first error from any tool
// or from yield (ErrStreamAborted); other goroutines are not explicitly cancelled.
func (r *Registry) ExecuteBatchStream(ctx context.Context, calls []ToolCall, yield func(Chunk) error) error {
	if len(calls) == 0 {
		return nil
	}
	var yieldMu sync.Mutex
	serializedYield := func(c Chunk) error {
		yieldMu.Lock()
		defer yieldMu.Unlock()
		return yield(c)
	}

	var firstErr error
	var firstErrMu sync.Mutex
	setFirstErr := func(err error) {
		if err == nil {
			return
		}
		firstErrMu.Lock()
		defer firstErrMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	var wg sync.WaitGroup
	for _, call := range calls {
		wg.Go(func() {
			toolYield := func(chunk []byte) error {
				// Check if another goroutine already failed (best-effort skip further work).
				firstErrMu.Lock()
				done := firstErr != nil
				firstErrMu.Unlock()
				if done {
					return ErrStreamAborted
				}
				return serializedYield(Chunk{CallID: call.ID, ToolName: call.ToolName, Data: chunk})
			}
			if err := r.Execute(ctx, call, toolYield); err != nil {
				setFirstErr(err)
			}
		})
	}
	wg.Wait()
	return firstErr
}

// Shutdown closes the registry for new calls and waits for in-flight executions or ctx to cancel.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return nil
	default:
		close(r.done)
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.running.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// panicError wraps a recovered panic value for SystemError; used by Registry and WithRecovery middleware.
type panicError struct{ p any }

func (e *panicError) Error() string {
	return "panic: " + fmt.Sprint(e.p)
}
