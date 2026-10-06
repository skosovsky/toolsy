package toolsy

import (
	"context"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
)

// SessionTrack stores session-level call admission state.
type SessionTrack struct {
	count    atomic.Int64
	maxCalls int64
}

func newSessionTrack(opts sessionOptions) *SessionTrack {
	return &SessionTrack{
		count:    atomic.Int64{},
		maxCalls: int64(opts.maxCalls),
	}
}

func (t *SessionTrack) consumeCallAttempt() error {
	if t == nil {
		return nil
	}
	attempt := t.count.Add(1)
	if t.maxCalls > 0 && attempt > t.maxCalls {
		return NewMaxCallsExceededError()
	}
	return nil
}

// CallAttempts counts outer Execute/RunCall admissions, including budget rejection,
// subsequent environment/registry/argument errors, cancellation and replay. A nil
// registry or RunPolicy rejection consumes nothing. Internal retries count once.
func (t *SessionTrack) CallAttempts() int64 {
	if t == nil {
		return 0
	}
	return t.count.Load()
}

// MaxCalls returns the configured call admission limit. Zero means unlimited.
func (t *SessionTrack) MaxCalls() int64 {
	if t == nil {
		return 0
	}
	return t.maxCalls
}

// Session is a stateful, concurrency-safe executor built on top of a stateless registry.
type Session struct {
	configuration atomic.Pointer[sessionConfiguration]
	track         *SessionTrack
	policy        RunPolicy
	opts          sessionOptions
	stateMu       sync.RWMutex
	state         map[string]any
}

// NewSession creates a session bound to reg and freezes its codec registry after
// run-policy/registry validation. Register every codec before creating a session;
// use a new codec registry to define a different schema.
func NewSession(reg *Registry, opts ...SessionOption) (*Session, error) {
	var cfg sessionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.policy = cloneRunPolicy(cfg.policy)
	if cfg.maxCalls < 0 {
		return nil, NewValidationError("maximum session calls must be nonnegative")
	}
	if err := ValidateRunPolicy(cfg.policy); err != nil {
		return nil, err
	}
	// Validate the registry before finalizing the caller's codec builder.
	bindingOptions := cfg
	bindingOptions.codecRegistry = nil
	binding, err := newSessionBinding(reg, bindingOptions)
	if err != nil {
		return nil, err
	}
	if err := validateRunPolicyCatalog(cfg.policy, binding.ToolNames); err != nil {
		return nil, err
	}
	cfg.codecRegistry.Freeze()
	binding.StateSchemaDigest = stateSchemaDigest(cfg.codecRegistry)
	storedOptions := cfg
	storedOptions.policy = cloneRunPolicy(cfg.policy)
	session := &Session{ //nolint:exhaustruct_v5 // Configuration/state locks have zero values; maps and pointer initialized below
		track:  newSessionTrack(cfg),
		policy: cloneRunPolicy(cfg.policy),
		opts:   storedOptions,
		state:  make(map[string]any),
	}
	session.configuration.Store(&sessionConfiguration{registry: reg, binding: binding})
	return session, nil
}

// Track returns the session execution track.
func (s *Session) Track() *SessionTrack {
	if s == nil {
		return nil
	}
	return s.track
}

// Execute runs one tool call through the session budget tracker.
// When call.Env is non-nil, it must be created with NewRunEnv(s) for this session (see ValidateRunEnvSession).
// call.Env may be nil for DI-only paths; SetState/GetState in tools are then no-ops — prefer NewRunEnv(s) for stateful tracks.
func (s *Session) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
	if s == nil {
		return NewToolNotFoundError()
	}
	return s.executeWithConfiguration(ctx, call, yield, s.executionConfiguration())
}

func (s *Session) executeWithConfiguration(
	ctx context.Context,
	call ToolCall,
	yield func(Chunk) error,
	configuration sessionConfiguration,
) error {
	if configuration.registry == nil {
		return NewToolNotFoundError()
	}
	if err := enforceRunPolicy(s.policy, call); err != nil {
		return err
	}
	if err := s.track.consumeCallAttempt(); err != nil {
		return err
	}
	if call.Env != nil {
		if err := ValidateRunEnvSession(s, call.Env); err != nil {
			return err
		}
	}
	return configuration.registry.execute(ctx, call, yield)
}

func enforceRunPolicy(p RunPolicy, call ToolCall) error {
	if p.ForcedTool != "" && call.ToolName != p.ForcedTool {
		return NewValidationError("tool " + call.ToolName + " is not the forced tool " + p.ForcedTool)
	}
	if len(p.AllowedTools) > 0 {
		if slices.Contains(p.AllowedTools, call.ToolName) {
			return nil
		}
		return NewValidationError("tool " + call.ToolName + " is not allowed by session run policy")
	}
	return nil
}

// ExecuteIter runs one tool call and returns an iterator over (Chunk, error) pairs.
func (s *Session) ExecuteIter(ctx context.Context, call ToolCall) iter.Seq2[Chunk, error] {
	return func(yield func(Chunk, error) bool) {
		ctxChild, cancel := context.WithCancel(ctx)
		defer cancel()
		var consumerStopped bool

		err := s.Execute(ctxChild, call, func(c Chunk) error {
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

		if !consumerStopped && err != nil && (!isContextInterrupt(err) || requiresOutcomeReconciliation(err)) {
			yield(Chunk{}, err)
		}
	}
}
