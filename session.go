package toolsy

import (
	"context"
	"errors"
	"iter"
	"slices"
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
		return ErrMaxStepsExceeded
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
type Session struct {
	reg    *Registry
	track  *SessionTrack
	policy RunPolicy
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
	return &Session{
		reg:    reg,
		track:  newSessionTrack(cfg),
		policy: cfg.policy,
	}, nil
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
		return ErrToolNotFound
	}
	if err := enforceRunPolicy(s.policy, call); err != nil {
		return err
	}
	if err := s.track.consumeStep(); err != nil {
		return err
	}
	return s.reg.execute(ctx, call, yield)
}

func enforceRunPolicy(p RunPolicy, call ToolCall) error {
	if p.ForcedTool != "" && call.ToolName != p.ForcedTool {
		return &ClientError{
			Reason:    "tool " + call.ToolName + " is not the forced tool " + p.ForcedTool,
			Retryable: false,
			Err:       ErrValidation,
		}
	}
	if len(p.AllowedTools) > 0 {
		if slices.Contains(p.AllowedTools, call.ToolName) {
			return nil
		}
		return &ClientError{
			Reason:    "tool " + call.ToolName + " is not allowed by session run policy",
			Retryable: false,
			Err:       ErrValidation,
		}
	}
	if len(p.RequiredTools) > 0 {
		if slices.Contains(p.RequiredTools, call.ToolName) {
			return nil
		}
		return &ClientError{
			Reason:    "tool " + call.ToolName + " is not in required tools for this session",
			Retryable: false,
			Err:       ErrValidation,
		}
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
