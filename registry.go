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

// RegistryBuilder is mutable setup API that produces an immutable Registry for runtime use.
type RegistryBuilder struct {
	tools       []Tool
	middlewares []Middleware
	opts        registryOptions
}

// NewRegistryBuilder creates a mutable registry builder with defaults and applies options.
func NewRegistryBuilder(opts ...RegistryOption) *RegistryBuilder {
	var o registryOptions
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
// Rejects tools with more than one [AsAsyncTool] layer anywhere in the chain (see [ChainUnwrapper]).
func (b *RegistryBuilder) Build() (*Registry, error) {
	tools := make(map[string]Tool, len(b.tools))
	for _, raw := range b.tools {
		if raw == nil {
			return nil, errors.New("toolsy: nil tool in registry builder")
		}
		t := raw
		if n := countAsyncLayers(t); n > 1 {
			return nil, fmt.Errorf(
				"toolsy: tool %q is wrapped in multiple AsAsyncTool layers, which is invalid",
				t.Manifest().Name,
			)
		}
		var asyncOpts *asyncOptions
		if aw, ok := t.(*asyncTool); ok {
			asyncOpts = &aw.opts
			t = aw.next
		}
		for i := len(b.middlewares) - 1; i >= 0; i-- {
			t = b.middlewares[i](t)
		}
		if asyncOpts != nil {
			t = &asyncTool{
				toolBase: toolBase{next: t},
				opts:     *asyncOpts,
			}
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
	return &Registry{
		tools: tools,
		opts:  b.opts,
		state: newRegistryRuntimeState(),
	}, nil
}

// countAsyncLayers walks toolBase chains and counts AsAsyncTool wrappers.
func countAsyncLayers(t Tool) int {
	n := 0
	for t != nil {
		if _, ok := t.(*asyncTool); ok {
			n++
		}
		u, ok := t.(ChainUnwrapper)
		if !ok {
			break
		}
		t = u.UnwrapNext()
	}
	return n
}

// registryRuntimeState is shared by a root registry and all views derived from it.
type registryRuntimeState struct {
	mu       sync.Mutex
	done     chan struct{}
	running  sync.WaitGroup
	closeMux sync.Once
}

func newRegistryRuntimeState() *registryRuntimeState {
	return &registryRuntimeState{
		mu:       sync.Mutex{},
		done:     make(chan struct{}),
		running:  sync.WaitGroup{},
		closeMux: sync.Once{},
	}
}

// tryStartExecution registers an in-flight execution unless the registry is shut down.
func (s *registryRuntimeState) tryStartExecution() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return false
	default:
		s.running.Add(1)
		return true
	}
}

// Registry holds tools and executes them with optional panic recovery.
type Registry struct {
	tools map[string]Tool
	opts  registryOptions
	state *registryRuntimeState
}

// NewRegistry creates an immutable registry from tools with default options.
func NewRegistry(tools ...Tool) (*Registry, error) {
	return NewRegistryBuilder().Add(tools...).Build()
}

func (r *Registry) requireRuntimeState() (*registryRuntimeState, error) {
	if r == nil || r.state == nil {
		return nil, NewRegistryStateError()
	}
	return r.state, nil
}

func (r *Registry) sortedToolNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// GetAllTools returns all registered tools, sorted by manifest name.
// This is a map-view helper only; it does not check runtime readiness.
// A nil receiver returns nil (no panic).
func (r *Registry) GetAllTools() []Tool {
	if r == nil {
		return nil
	}
	names := r.sortedToolNames()
	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

// GetTool returns the tool with the given name, or (nil, false) if not found.
// A nil receiver returns (nil, false).
func (r *Registry) GetTool(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

// Has reports whether a tool with the given name is registered in this view's tool map.
// It does not check runtime readiness; use [ValidateManifestContract] or [Registry.Execute] for that.
// A nil receiver returns false.
func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.tools[name]
	return ok
}

// ManifestSet returns a declarative manifest view of registered tools (no runtime state required).
func (r *Registry) ManifestSet() (ManifestSet, error) {
	if r == nil || len(r.tools) == 0 {
		return ManifestSet{}, nil
	}
	return manifestSetFromToolMap(r.tools)
}

// ToolNames returns all registered tool names, sorted lexicographically.
// Useful for prompt manifests and contract validation.
// This is a map-view helper only; it does not check runtime readiness.
// A nil receiver returns nil (no panic).
func (r *Registry) ToolNames() []string {
	return r.sortedToolNames()
}

// Subset returns a capability-backed registry view containing only the named tools from r.
// Duplicate names in allowedNames are ignored (silent dedup).
// Returns an error if any name is not present in r (strict fail-fast).
// The parent registry is not modified. Registry options (hooks, validator) are inherited.
// Subset shares runtime state (Shutdown, in-flight tracking) with r and all sibling views.
// Shutdown on either parent or subset stops the entire tree (see [Registry.Shutdown]).
// Prefer [Registry.View] when the caller needs snapshot identity, required tool validation, or view policy.
func (r *Registry) Subset(allowedNames ...string) (*Registry, error) {
	view, err := r.View(RegistryViewSpec{
		ToolNames:         allowedNames,
		RequiredToolNames: nil,
		Reason:            "registry subset",
		Owner:             "toolsy",
		Policy:            nil,
	})
	if err != nil {
		return nil, err
	}
	return view.reg, nil
}

func (r *Registry) subsetWithPolicy(policy Policy, allowedNames ...string) (*Registry, error) {
	if _, err := r.requireRuntimeState(); err != nil {
		return nil, fmt.Errorf("toolsy: subset: %w", err)
	}
	opts := r.opts
	opts.policy = composePolicies(opts.policy, policy)
	seen := make(map[string]struct{}, len(allowedNames))
	tools := make(map[string]Tool, len(allowedNames))
	for _, name := range allowedNames {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		tool, ok := r.tools[name]
		if !ok {
			return nil, NewToolNotFoundInSubsetError(name)
		}
		tools[name] = tool
	}
	return &Registry{
		tools: tools,
		opts:  opts,
		state: r.state,
	}, nil
}

// accountDeliveredChunk updates ExecutionSummary after a chunk was successfully yielded to the consumer.
func (r *Registry) accountDeliveredChunk(ctx context.Context, c Chunk, summary *ExecutionSummary) {
	if c.IsError {
		summary.ErrorChunks++
		summary.LastErrorText = errorChunkSummaryText(c, nil)
		return
	}
	summary.ChunksDelivered++
	summary.TotalBytes += int64(len(c.Data))
	if r.opts.onChunk != nil {
		r.opts.onChunk(ctx, c)
	}
}

// wrapYieldWithCallMeta fills CallID/ToolName, validates chunks, updates summary counters,
// and invokes onChunk for delivered non-error chunks.
func (r *Registry) wrapYieldWithCallMeta(
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
			c.CallID = call.Input.CallID
		}
		if c.ToolName == "" {
			c.ToolName = call.ToolName
		}
		prepared, err := prepareChunk(c)
		if err != nil {
			return err
		}
		c = prepared
		yieldErr := yield(c)
		if yieldErr != nil {
			return yieldErr
		}
		r.accountDeliveredChunk(ctx, c, summary)
		return nil
	}
}

// Execute runs one tool call and streams chunks to yield. Returns on first yield error or tool error.
// The after-execution hook (WithOnAfterExecute) is always invoked via defer with ExecutionSummary.
// ChunksDelivered and TotalBytes count only chunks with !IsError. ErrorChunks/LastErrorText
// describe delivered soft-error chunks.
//
// Execute does not validate that call.Env is bound to a [Session]. For stateful agent tracks use
// [Session.Execute] or call [ValidateRunEnvSession] before Execute when env must match a session.
func (r *Registry) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
	return r.execute(ctx, call, yield)
}

func (r *Registry) execute(
	ctx context.Context,
	call ToolCall,
	yield func(Chunk) error,
) error {
	_, _, err := r.executeWithSummary(ctx, call, yield, true)
	return err
}

// executeWithSummary runs a single tool call with hooks and optional panic recovery.
// Named result err is required: after recover() stops a panic, Go does not run the final return
// statement, so the error must be assigned from a defer (see TestRegistry_Execute_PanicRecovery_OnAfterSummary).
//
//nolint:nonamedreturns // panic recovery requires a named error result; plain returns stay nil after recover.
func (r *Registry) executeWithSummary(
	ctx context.Context,
	call ToolCall,
	yield func(Chunk) error,
	withAfterHook bool,
) (summary ExecutionSummary, summaryReady bool, err error) {
	state, stateErr := r.requireRuntimeState()
	if stateErr != nil {
		return summary, false, stateErr
	}
	if !state.tryStartExecution() {
		return summary, false, NewShutdownError()
	}
	tool, ok := r.tools[call.ToolName]
	if !ok {
		state.running.Done()
		if r.opts.view.ID != "" {
			return summary, false, NewCapabilityDeniedError(call.ToolName, r.opts.view)
		}
		return summary, false, NewToolNotFoundError()
	}

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { state.running.Done() }) }
	execEnv := call.Env
	if execEnv == nil {
		execEnv = NewRunEnv(nil)
	}
	call.Input = call.Input.Clone()
	call.CallContext = bindCallMetadata(call.CallContext, call)
	if r.opts.view.ID != "" {
		call.CallContext = bindViewMetadata(call.CallContext, r.opts.view.ID)
	}
	execEnv = execEnv.cloneForExecute(call.Input.Attachments, newAsyncRuntime(r), call.CallContext)
	execEnv.view = cloneRegistryViewSnapshot(r.opts.view)
	defer func() {
		if execEnv.async == nil || !execEnv.async.backgroundStarted.Load() {
			release()
		}
	}()

	summary.CallID = call.Input.CallID
	summary.ToolName = call.ToolName
	summaryReady = true
	start := time.Now()
	if withAfterHook {
		defer func() {
			dur := time.Since(start)
			if r.opts.onAfter != nil {
				r.opts.onAfter(ctx, cloneToolCall(call), summary, dur)
			}
		}()
	}
	if r.opts.recoverPanics {
		defer func() {
			if p := recover(); p != nil {
				summary.Error = NewInternalError(&panicError{p: p})
				err = summary.Error
			}
		}()
	}

	if r.opts.onBefore != nil {
		r.opts.onBefore(ctx, cloneToolCall(call))
	}

	toolYield := r.wrapYieldWithCallMeta(ctx, call, &summary, yield)
	r.runToolWithValidationAndExecute(ctx, call, execEnv, tool, toolYield, &summary)
	err = summary.Error
	return summary, summaryReady, err
}

// runToolWithValidationAndExecute runs optional validator then tool.Execute; maps DeadlineExceeded to ErrTimeout.
func (r *Registry) runToolWithValidationAndExecute(
	ctx context.Context,
	call ToolCall,
	env *RunEnv,
	tool Tool,
	toolYield func(Chunk) error,
	summary *ExecutionSummary,
) {
	manifest := cloneManifestForPolicy(tool.Manifest())
	if err := enforceRequirementsPolicy(manifest.Requirements, r.opts.policy); err != nil {
		summary.Error = err
		return
	}
	if r.opts.authorizer != nil {
		req := AuthorizationRequest{
			Manifest:    cloneManifestForPolicy(manifest),
			Input:       call.Input.Clone(),
			CallContext: call.CallContext,
			View:        cloneRegistryViewSnapshot(r.opts.view),
		}
		if aErr := r.opts.authorizer.Authorize(ctx, req); aErr != nil {
			if _, ok := AsToolError(aErr); ok {
				summary.Error = aErr
			} else {
				summary.Error = NewPolicyDeniedErrorFrom(aErr)
			}
			return
		}
	}
	if r.opts.policy != nil {
		req := PolicyRequest{
			Manifest:    cloneManifestForPolicy(manifest),
			Input:       call.Input.Clone(),
			CallContext: call.CallContext,
			View:        cloneRegistryViewSnapshot(r.opts.view),
		}
		if pErr := evaluatePolicy(ctx, r.opts.policy, req); pErr != nil {
			summary.Error = pErr
			return
		}
	}
	if err := enforceRuntimeRequirements(manifest.Requirements, env); err != nil {
		summary.Error = err
		return
	}
	if r.opts.validator != nil {
		if vErr := r.opts.validator.Validate(ctx, call.ToolName, string(call.Input.ArgsJSON)); vErr != nil {
			summary.Error = NewValidationError(
				"tool execution failed: security validation failed: " + vErr.Error() + ". Please fix the arguments and try again.",
			)
			return
		}
	}
	summary.Error = tool.Execute(ctx, env, call.Input, toolYield)
	summary.Error = normalizeExecutionInterrupt(summary.Error)
}

func enforceRequirementsPolicy(req ToolRequirements, policy Policy) error {
	if !hasRequirements(req) || policyEnforcesRequirements(policy) {
		return nil
	}
	return NewPolicyDeniedError("tool requirements require a requirements policy", "requirements")
}

func enforceRuntimeRequirements(req ToolRequirements, env *RunEnv) error {
	if !req.NeedsSession && (req.MemoryAccess == "" || req.MemoryAccess == MemoryAccessNone) {
		return nil
	}
	if env != nil && (env.session != nil || env.StateStore != nil) {
		return nil
	}
	return NewDependencyMissingError("session state")
}

func normalizeExecutionInterrupt(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if !isContextInterrupt(err) {
		return err
	}
	if te, ok := AsToolError(err); ok && te.Code == CodeTimeout {
		return err
	}
	return NewTimeoutErrorFrom(err, true)
}

// ExecuteIter runs one tool call and returns an iterator over (Chunk, error) pairs.
// Push-to-push: no channels or extra goroutines; the iterator calls Execute with a callback that forwards to yield.
// When the consumer breaks out of the loop, cancel() is called and Execute exits via [context.Canceled].
// Once yield returns false, the iterator must not call yield again (iter contract).
// Env/session binding: same as [Registry.Execute] (no automatic ValidateRunEnvSession).
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

		if !consumerStopped && err != nil && !isContextInterrupt(err) {
			yield(Chunk{}, err)
		}
	}
}

// batchYieldGate serializes batch stream delivery to the user yield under abort/cancel rules.
type batchYieldGate struct {
	yieldMu     sync.Mutex
	batchCtx    context.Context
	yield       func(Chunk) error
	getAbortErr func() error
	recordAbort func(error)
}

func (g *batchYieldGate) abortOrBatchDone() error {
	if abortErr := g.getAbortErr(); abortErr != nil {
		return abortErr
	}
	if err := g.batchCtx.Err(); err != nil {
		return err
	}
	return nil
}

func (g *batchYieldGate) safeYield(c Chunk) error {
	if err := g.abortOrBatchDone(); err != nil {
		return err
	}
	g.yieldMu.Lock()
	defer g.yieldMu.Unlock()
	if err := g.abortOrBatchDone(); err != nil {
		return err
	}
	if yieldErr := g.yield(c); yieldErr != nil {
		abortErr := wrapYieldError(yieldErr)
		g.recordAbort(abortErr)
		return abortErr
	}
	return nil
}

func (r *Registry) handleBatchToolError(
	call ToolCall,
	execErr error,
	summary *ExecutionSummary,
	summaryReady bool,
	safeYield func(Chunk) error,
	recordStreamAbort func(error),
	suspendErr *error,
	suspendMu *sync.Mutex,
) {
	if execErr == nil {
		return
	}
	switch {
	case IsControlError(execErr):
		suspendMu.Lock()
		if *suspendErr == nil {
			*suspendErr = execErr
		}
		suspendMu.Unlock()
	case errors.Is(execErr, ErrStreamAborted):
		recordStreamAbort(execErr)
	case isContextInterrupt(execErr):
	default:
		errChunk := NewErrorChunkFromErr(execErr)
		prepared, prepErr := prepareChunk(errChunk)
		// Defensive: NewErrorChunkFromErr always marshals recoverable wire today; prepErr guards future validate changes.
		if prepErr != nil {
			_ = safeYield(NewErrorChunkFromErr(prepErr))
			return
		}
		errChunk = prepared
		errChunk.CallID = call.Input.CallID
		errChunk.ToolName = call.ToolName
		yieldErr := safeYield(errChunk)
		if yieldErr == nil {
			if summaryReady {
				summary.Error = nil
				summary.ErrorChunks++
				summary.LastErrorText = errorChunkSummaryText(errChunk, execErr)
			}
			return
		}
		if errors.Is(yieldErr, ErrStreamAborted) {
			recordStreamAbort(yieldErr)
		}
	}
}

func (r *Registry) runBatchStreamWorker(
	batchCtx context.Context,
	call ToolCall,
	gate *batchYieldGate,
	recordStreamAbort func(error),
	suspendErr *error,
	suspendMu *sync.Mutex,
) {
	start := time.Now()
	var summary ExecutionSummary
	var summaryReady bool
	defer func() {
		if !summaryReady || r.opts.onAfter == nil {
			return
		}
		r.opts.onAfter(batchCtx, cloneToolCall(call), summary, time.Since(start))
	}()
	toolYield := func(c Chunk) error {
		if c.CallID == "" {
			c.CallID = call.Input.CallID
		}
		if c.ToolName == "" {
			c.ToolName = call.ToolName
		}
		return gate.safeYield(c)
	}
	execSummary, ready, err := r.executeWithSummary(batchCtx, call, toolYield, false)
	summary = execSummary
	summaryReady = ready
	r.handleBatchToolError(call, err, &summary, summaryReady, gate.safeYield, recordStreamAbort, suspendErr, suspendMu)
}

// ExecuteBatchStream runs all calls in parallel and streams chunks via yield. Each chunk is
// tagged with CallID and ToolName. Non-suspend execution failures (including pre-tool dispatch
// errors and tool/middleware failures) are sent as Chunk with IsError: true; the method returns
// error only for critical failures (context canceled, stream aborted, suspend). After [Registry.Shutdown],
// new calls receive a soft error chunk (IsError: true) per call, not [ErrShutdown] from ExecuteBatchStream itself.
// For [AsAsyncTool], batch yields sync chunks (typically AsyncAccepted) and returns while background work
// continues; [Registry.Shutdown] still waits for those background jobs via the async runtime tracker.
// The library serializes calls to yield with a mutex so the caller's callback need not be thread-safe.
func (r *Registry) ExecuteBatchStream(ctx context.Context, calls []ToolCall, yield func(Chunk) error) error {
	if len(calls) == 0 {
		return nil
	}
	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var suspendErr error
	var suspendMu sync.Mutex
	var streamAbortErr error
	var streamAbortOnce sync.Once
	var streamAbortMu sync.RWMutex
	getStreamAbortErr := func() error {
		streamAbortMu.RLock()
		defer streamAbortMu.RUnlock()
		return streamAbortErr
	}
	recordStreamAbort := func(err error) {
		if err == nil {
			return
		}
		streamAbortOnce.Do(func() {
			streamAbortMu.Lock()
			streamAbortErr = err
			streamAbortMu.Unlock()
			cancel()
		})
	}
	gate := &batchYieldGate{
		yieldMu:     sync.Mutex{},
		batchCtx:    batchCtx,
		yield:       yield,
		getAbortErr: getStreamAbortErr,
		recordAbort: recordStreamAbort,
	}
	for _, call := range calls {
		c := call
		wg.Go(func() {
			r.runBatchStreamWorker(batchCtx, c, gate, recordStreamAbort, &suspendErr, &suspendMu)
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if streamAbortErr := getStreamAbortErr(); streamAbortErr != nil {
		return streamAbortErr
	}
	if suspendErr != nil {
		return suspendErr
	}
	return nil
}

// Shutdown closes the registry for new calls and waits for in-flight executions or ctx to cancel.
// Both synchronous executions and background jobs started by AsAsyncTool (when run via Registry) are tracked;
// Shutdown blocks until all of them finish or ctx is cancelled.
//
// Registry views share runtime state with their parent. Calling Shutdown on any view closes the shared
// lifecycle for the entire registry tree (idempotent via [sync.Once]). Only the application owner of the
// root registry (for example App or Server on SIGTERM) should call Shutdown; do not call Shutdown on a
// per-request view to "clean up" after an agent run.
func (r *Registry) Shutdown(ctx context.Context) error {
	state, err := r.requireRuntimeState()
	if err != nil {
		return err
	}
	state.closeMux.Do(func() { close(state.done) })
	done := make(chan struct{})
	go func() {
		state.running.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// panicError wraps a recovered panic value for internal [ToolError].
type panicError struct{ p any }

func (e *panicError) Error() string {
	return "panic: " + fmt.Sprint(e.p)
}
