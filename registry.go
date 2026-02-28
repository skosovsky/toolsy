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
	tools       map[string]Tool
	rawTools    map[string]Tool
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

// GetAllTools returns all registered tools, sorted by name.
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

// GetTool returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) GetTool(name string) (Tool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tools[name]
	return t, ok
}

// Execute runs one tool call and streams chunks to yield. Returns on first yield error or tool error.
// The after-execution hook (WithOnAfterExecute) is always invoked via defer with ExecutionSummary.
// ChunksDelivered and TotalBytes count only chunks with !IsError.
func (r *Registry) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) (err error) {
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

	// Wrap yield: fill CallID/ToolName, count only !IsError chunks, call onChunk for successfully delivered non-error chunks.
	toolYield := func(c Chunk) error {
		if c.CallID == "" {
			c.CallID = call.ID
		}
		if c.ToolName == "" {
			c.ToolName = call.ToolName
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

	summary.Error = tool.Execute(ctx, call.Args, toolYield)
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
// tagged with CallID and ToolName. Tool errors are sent as Chunk with IsError: true; the method
// returns error only for critical failures (context cancelled, shutdown). The library serializes
// calls to yield with a mutex so the caller's callback need not be thread-safe.
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
				// Send tool error as chunk; do not return it from ExecuteBatchStream.
				_ = safeYield(Chunk{
					CallID:   call.ID,
					ToolName: call.ToolName,
					Event:    EventResult,
					Data:     []byte(err.Error()),
					IsError:  true,
				})
			}
		})
	}
	wg.Wait()
	// Return only critical failures to avoid goroutine leaks and to let callers see all chunks.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
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

// panicError wraps a recovered panic value for SystemError.
type panicError struct{ p any }

func (e *panicError) Error() string {
	return "panic: " + fmt.Sprint(e.p)
}
