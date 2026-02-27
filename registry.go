package toolsy

import (
	"context"
	"encoding/json"
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

// Execute runs one tool call. Partial Success: each call returns independently; failures do not cancel others.
func (r *Registry) Execute(ctx context.Context, call ToolCall) (result ToolResult) {
	result = ToolResult{CallID: call.ID, ToolName: call.ToolName}
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		result.Error = ErrShutdown
		return result
	default:
	}
	tool, ok := r.tools[call.ToolName]
	if !ok {
		r.mu.Unlock()
		result.Error = ErrToolNotFound
		return result
	}
	r.running.Add(1)
	r.mu.Unlock()

	if err := r.acquireSemaphore(ctx); err != nil {
		r.running.Done()
		if errors.Is(err, context.DeadlineExceeded) {
			result.Error = ErrTimeout
		} else {
			result.Error = err
		}
		return result
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

	if r.opts.recoverPanics {
		defer func() {
			if p := recover(); p != nil {
				result.Error = &SystemError{Err: &panicError{p: p}}
			}
		}()
	}

	if r.opts.onBefore != nil {
		r.opts.onBefore(ctx, call)
	}
	start := time.Now()
	res, err := tool.Execute(ctx, call.Args)
	dur := time.Since(start)
	if r.opts.onAfter != nil {
		r.opts.onAfter(ctx, call, ToolResult{CallID: call.ID, ToolName: call.ToolName, Result: json.RawMessage(res), Error: err}, dur)
	}
	if err != nil {
		result.Error = err
		return result
	}
	result.Result = json.RawMessage(res)
	return result
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

// ExecuteBatch runs all calls in parallel and collects all results (Partial Success).
func (r *Registry) ExecuteBatch(ctx context.Context, calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(idx int, call ToolCall) {
			defer wg.Done()
			results[idx] = r.Execute(ctx, call)
		}(i, c)
	}
	wg.Wait()
	return results
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
