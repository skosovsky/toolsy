package toolsy

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
	"time"
)

const (
	defaultRegistryTimeout   = 5 * time.Second
	defaultRegistrySemaphore = 10
)

// RegistryBuilder is mutable setup API that produces an immutable Registry for runtime use.
type RegistryBuilder struct {
	tools       []Tool
	middlewares []Middleware
	opts        registryOptions
}

// NewRegistryBuilder creates a mutable registry builder with defaults and applies options.
func NewRegistryBuilder(opts ...RegistryOption) *RegistryBuilder {
	var o registryOptions
	o.timeout = defaultRegistryTimeout
	o.maxConcurrency = defaultRegistrySemaphore
	o.recoverPanics = true
	for _, opt := range opts {
		opt(&o)
	}
	return &RegistryBuilder{
		tools:       nil,
		middlewares: nil,
		opts:        o,
	}
}

// Add appends tools to the builder.
func (b *RegistryBuilder) Add(tools ...Tool) *RegistryBuilder {
	b.tools = append(b.tools, tools...)
	return b
}

// Use appends middlewares. The first middleware is outermost.
func (b *RegistryBuilder) Use(middlewares ...Middleware) *RegistryBuilder {
	b.middlewares = append(b.middlewares, middlewares...)
	return b
}

// WithOptions applies registry options to the builder.
func (b *RegistryBuilder) WithOptions(opts ...RegistryOption) *RegistryBuilder {
	for _, opt := range opts {
		opt(&b.opts)
	}
	return b
}

// Build creates an immutable runtime registry.
func (b *RegistryBuilder) Build() (*Registry, error) {
	tools := make(map[string]Tool, len(b.tools))
	for _, raw := range b.tools {
		if raw == nil {
			return nil, errors.New("toolsy: nil tool in registry builder")
		}
		t := raw
		for i := len(b.middlewares) - 1; i >= 0; i-- {
			t = b.middlewares[i](t)
		}
		name := t.Manifest().Name
		if name == "" {
			return nil, errors.New("toolsy: tool manifest name is required")
		}
		if _, exists := tools[name]; exists {
			return nil, fmt.Errorf("toolsy: duplicate tool name %q", name)
		}
		tools[name] = t
	}
	var sem chan struct{}
	if b.opts.maxConcurrency > 0 {
		sem = make(chan struct{}, b.opts.maxConcurrency)
	}
	return &Registry{
		tools:    tools,
		sem:      sem,
		opts:     b.opts,
		done:     make(chan struct{}),
		running:  sync.WaitGroup{},
		closeMux: sync.Once{},
	}, nil
}

// Registry holds tools and executes them with timeout, semaphore, and optional panic recovery.
type Registry struct {
	tools   map[string]Tool
	sem     chan struct{}
	opts    registryOptions
	done    chan struct{}
	running sync.WaitGroup

	closeMux sync.Once
}

// NewRegistry creates an immutable registry from tools with default options.
func NewRegistry(tools ...Tool) (*Registry, error) {
	return NewRegistryBuilder().Add(tools...).Build()
}

// GetAllTools returns all registered tools, sorted by manifest name.
func (r *Registry) GetAllTools() []Tool {
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

// GetTool returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) GetTool(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// effectiveExecuteTimeout is the minimum of registry default and per-tool manifest timeout when both are set.
func effectiveExecuteTimeout(tool Tool, registryTimeout time.Duration) time.Duration {
	timeout := registryTimeout
	toolTimeout := tool.Manifest().Timeout
	if toolTimeout > 0 && (registryTimeout <= 0 || toolTimeout < registryTimeout) {
		timeout = toolTimeout
	}
	return timeout
}

// wrapYieldWithMetadata fills CallID/ToolName, validates chunks, counts successful chunks, and invokes onChunk.
func (r *Registry) wrapYieldWithMetadata(
	ctx context.Context,
	call ToolCall,
	summary *ExecutionSummary,
	yield func(Chunk) error,
) func(Chunk) error {
	return func(c Chunk) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.CallID == "" {
			c.CallID = call.ID
		}
		if c.ToolName == "" {
			c.ToolName = call.ToolName
		}
		if err := validateChunk(c); err != nil {
			return err
		}
		yieldErr := yield(c)
		if yieldErr == nil && !c.IsError {
			summary.ChunksDelivered++
			summary.TotalBytes += int64(len(c.Data))
			if r.opts.onChunk != nil {
				r.opts.onChunk(ctx, c)
			}
		}
		return yieldErr
	}
}

// Execute runs one tool call and streams chunks to yield. Returns on first yield error or tool error.
// The after-execution hook (WithOnAfterExecute) is always invoked via defer with ExecutionSummary.
// ChunksDelivered and TotalBytes count only chunks with !IsError.
func (r *Registry) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
	return r.execute(ctx, call, yield)
}

func (r *Registry) execute(
	ctx context.Context,
	call ToolCall,
	yield func(Chunk) error,
) (err error) {
	select {
	case <-r.done:
		return ErrShutdown
	default:
	}
	tool, ok := r.tools[call.ToolName]
	if !ok {
		return ErrToolNotFound
	}
	r.running.Add(1)

	// Apply effective timeout before acquireSemaphore so queue wait is within the timeout budget.
	timeout := effectiveExecuteTimeout(tool, r.opts.timeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err = r.acquireSemaphore(ctx); err != nil {
		r.running.Done()
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrTimeout
		}
		return err
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { r.releaseSemaphore(); r.running.Done() }) }
	run := call.Run
	run.attachments = cloneAttachments(call.Input.Attachments)
	run.async = newAsyncRuntime(r, timeout)
	defer func() {
		if run.async == nil || !run.async.backgroundStarted.Load() {
			release()
		}
	}()

	var summary ExecutionSummary
	summary.CallID = call.ID
	summary.ToolName = call.ToolName
	start := time.Now()
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

	toolYield := r.wrapYieldWithMetadata(ctx, call, &summary, yield)
	r.runToolWithValidationAndExecute(ctx, call, run, tool, toolYield, &summary)
	return summary.Error
}

// runToolWithValidationAndExecute runs optional validator then tool.Execute; maps DeadlineExceeded to ErrTimeout.
func (r *Registry) runToolWithValidationAndExecute(
	ctx context.Context,
	call ToolCall,
	run RunContext,
	tool Tool,
	toolYield func(Chunk) error,
	summary *ExecutionSummary,
) {
	if r.opts.validator != nil {
		if vErr := r.opts.validator.Validate(ctx, call.ToolName, string(call.Input.ArgsJSON)); vErr != nil {
			summary.Error = &ClientError{
				Reason:    "tool execution failed: security validation failed: " + vErr.Error() + ". Please fix the arguments and try again.",
				Retryable: false,
				Err:       ErrValidation,
			}
			return
		}
	}
	summary.Error = tool.Execute(ctx, run, call.Input, toolYield)
	if errors.Is(summary.Error, context.DeadlineExceeded) {
		summary.Error = ErrTimeout
	}
}

// ExecuteIter runs one tool call and returns an iterator over (Chunk, error) pairs.
// Push-to-push: no channels or extra goroutines; the iterator calls Execute with a callback that forwards to yield.
// When the consumer breaks out of the loop, cancel() is called and Execute exits via [context.Canceled].
// Once yield returns false, the iterator must not call yield again (iter contract).
func (r *Registry) ExecuteIter(ctx context.Context, call ToolCall) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		ctxChild, cancel := context.WithCancel(ctx)
		defer cancel()
		var consumerStopped bool

		err := r.execute(ctxChild, call, func(c Chunk) error {
			if consumerStopped {
				return context.Canceled
			}
			if !yield(c, nil) {
				consumerStopped = true
				cancel()
				return context.Canceled
			}
			return nil
		})

		if !consumerStopped && err != nil && !errors.Is(err, context.Canceled) {
			yield(Chunk{}, err)
		}
	}
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
// tagged with CallID and ToolName. Tool errors are sent as Chunk with IsError: true; the method
// returns error only for critical failures (context cancelled, shutdown). The library serializes
// calls to yield with a mutex so the caller's callback need not be thread-safe.
//
//nolint:gocognit
func (r *Registry) ExecuteBatchStream(ctx context.Context, calls []ToolCall, yield func(Chunk) error) error {
	if len(calls) == 0 {
		return nil
	}
	var yieldMu sync.Mutex
	safeYield := func(c Chunk) error {
		yieldMu.Lock()
		defer yieldMu.Unlock()
		return yield(c)
	}

	var wg sync.WaitGroup
	var suspendErr error
	var suspendMu sync.Mutex
	for _, call := range calls {
		wg.Go(func() {
			toolYield := func(c Chunk) error {
				if c.CallID == "" {
					c.CallID = call.ID
				}
				if c.ToolName == "" {
					c.ToolName = call.ToolName
				}
				return safeYield(c)
			}
			if err := r.Execute(ctx, call, toolYield); err != nil {
				if errors.Is(err, ErrSuspend) {
					suspendMu.Lock()
					if suspendErr == nil {
						suspendErr = err
					}
					suspendMu.Unlock()
					return
				}
				_ = safeYield(Chunk{
					CallID:   call.ID,
					ToolName: call.ToolName,
					Event:    EventResult,
					Data:     []byte(err.Error()),
					MimeType: MimeTypeText,
					IsError:  true,
				})
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if suspendErr != nil {
		return suspendErr
	}
	return nil
}

// Shutdown closes the registry for new calls and waits for in-flight executions or ctx to cancel.
// Both synchronous executions and background jobs started by AsAsyncTool (when run via Registry) are tracked;
// Shutdown blocks until all of them finish or ctx is cancelled.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.closeMux.Do(func() { close(r.done) })
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

// panicError wraps a recovered panic value for SystemError.
type panicError struct{ p any }

func (e *panicError) Error() string {
	return "panic: " + fmt.Sprint(e.p)
}
