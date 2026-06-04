package toolsy

import (
	"context"
	"errors"
	"iter"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// SessionTrack stores session-level execution budget state.
type SessionTrack struct {
	count    atomic.Int64
	maxSteps int64
}

func newSessionTrack(opts sessionOptions) *SessionTrack {
	return &SessionTrack{
		count:    atomic.Int64{},
		maxSteps: int64(opts.maxSteps),
	}
}

func (t *SessionTrack) consumeStep() error {
	if t == nil {
		return nil
	}
	step := t.count.Add(1)
	if t.maxSteps > 0 && step > t.maxSteps {
		return NewMaxStepsExceededError()
	}
	return nil
}

// ExecutionCount returns the number of consumed session steps (outer Session.Execute calls).
// Internal retry attempts are not counted separately.
func (t *SessionTrack) ExecutionCount() int64 {
	if t == nil {
		return 0
	}
	return t.count.Load()
}

// MaxSteps returns the configured execution budget for this track. Zero means unlimited.
func (t *SessionTrack) MaxSteps() int64 {
	if t == nil {
		return 0
	}
	return t.maxSteps
}

// Session is a stateful, concurrency-safe executor built on top of a stateless registry.
// It owns mutable in-memory session state; [RunEnv] holds per-call dependencies only.
type Session struct {
	reg    *Registry
	track  *SessionTrack
	policy RunPolicy
	opts   sessionOptions

	stateMu sync.RWMutex
	state   map[string]any
}

// NewSession creates a new session bound to reg.
func NewSession(reg *Registry, opts ...SessionOption) (*Session, error) {
	var cfg sessionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := ValidateRunPolicy(cfg.policy); err != nil {
		return nil, err
	}
	return &Session{ //nolint:exhaustruct // stateMu zero value; state map initialized below
		reg:    reg,
		track:  newSessionTrack(cfg),
		policy: cfg.policy,
		opts:   cfg,
		state:  make(map[string]any),
	}, nil
}

// Export returns a shallow copy of in-memory session state suitable for JSON serialization.
// Dependencies, attachments, and [StateStore] data are not included.
// Export on a nil receiver returns nil; an empty session returns a non-nil empty map.
func (s *Session) Export() map[string]any {
	if s == nil {
		return nil
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	out := make(map[string]any, len(s.state))
	maps.Copy(out, s.state)
	return out
}

// Import replaces in-memory session state from data (e.g. after JSON unmarshal).
// Registered keys in [WithStateTypeRegistry] are restored to concrete Go types.
// Import(nil) clears all session state keys.
func (s *Session) Import(data map[string]any) error {
	if s == nil {
		return NewValidationError("nil session")
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	next := make(map[string]any, len(data))
	for k, v := range data {
		if v == nil {
			next[k] = nil
			continue
		}
		decoded, err := importStateValue(s.opts.stateRegistry, k, v)
		if err != nil {
			return err
		}
		next[k] = decoded
	}
	s.state = next
	return nil
}

// Track returns the session execution track.
func (s *Session) Track() *SessionTrack {
	if s == nil {
		return nil
	}
	return s.track
}

// Execute runs one tool call through the session budget tracker.
func (s *Session) Execute(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
	if s == nil || s.reg == nil {
		return NewToolNotFoundError()
	}
	if err := validateSessionRunEnv(s, call.Env); err != nil {
		return err
	}
	if err := enforceRunPolicy(s.policy, call); err != nil {
		return err
	}
	if err := s.track.consumeStep(); err != nil {
		return err
	}
	return s.reg.execute(ctx, call, yield)
}

// ValidateRunEnvSession checks that env is nil, DI-only (no session), or bound to s.
// Use before [Registry.Execute] when the track uses in-memory session state; [Session.Execute] calls this internally.
func ValidateRunEnvSession(s *Session, env *RunEnv) error {
	return validateSessionRunEnv(s, env)
}

func validateSessionRunEnv(s *Session, env *RunEnv) error {
	if env == nil || env.session == nil {
		return nil
	}
	if s == nil {
		return NewValidationError("nil session")
	}
	if env.session != s {
		return NewValidationError("session/env mismatch: ToolCall.Env is bound to a different Session")
	}
	return nil
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
	if len(p.RequiredTools) > 0 {
		if slices.Contains(p.RequiredTools, call.ToolName) {
			return nil
		}
		return NewValidationError("tool " + call.ToolName + " is not in required tools for this session")
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

		if !consumerStopped && err != nil && !errors.Is(err, context.Canceled) {
			yield(Chunk{}, err)
		}
	}
}
