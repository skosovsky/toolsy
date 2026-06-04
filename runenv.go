package toolsy

import (
	"sync"
)

// DepKeyBudget is the dependency map key for [BudgetTracker] used by [WithBudget].
const DepKeyBudget = "toolsy.budget"

// runEnvStore holds per-run dependency maps; clones of [RunEnv] share the same store pointer.
type runEnvStore struct {
	mu   sync.RWMutex
	deps map[string]any
}

func newRunEnvStore() *runEnvStore {
	return &runEnvStore{
		mu:   sync.RWMutex{},
		deps: make(map[string]any),
	}
}

// RunEnv is the per-call execution environment: credentials, [StateStore], and keyed dependencies.
// Mutable in-memory session state lives on [Session]; wire via [NewRunEnv](session).
type RunEnv struct {
	Credentials CredentialsProvider
	StateStore  StateStore

	session     *Session
	store       *runEnvStore
	attachments []Attachment
	async       *asyncRuntime
}

// RunEnvOption configures [NewRunEnv].
type RunEnvOption func(*RunEnv)

// WithCredentials sets the credentials provider on the environment.
func WithCredentials(p CredentialsProvider) RunEnvOption {
	return func(e *RunEnv) {
		e.Credentials = p
	}
}

// WithStateStore sets the persisted state store on the environment.
func WithStateStore(s StateStore) RunEnvOption {
	return func(e *RunEnv) {
		e.StateStore = s
	}
}

// NewRunEnv creates a run environment bound to session (may be nil for DI-only tests).
// State mutations require a non-nil session; use [SetSessionState] from the host when needed.
func NewRunEnv(session *Session, opts ...RunEnvOption) *RunEnv {
	env := &RunEnv{ //nolint:exhaustruct // optional providers set via RunEnvOption
		session: session,
		store:   newRunEnvStore(),
	}
	for _, opt := range opts {
		opt(env)
	}
	return env
}

// Session returns the session this environment is bound to, or nil for DI-only envs.
func (e *RunEnv) Session() *Session {
	if e == nil {
		return nil
	}
	return e.session
}

// Attachments returns runtime attachments for the current call (cloned).
func (e *RunEnv) Attachments() []Attachment {
	if e == nil {
		return nil
	}
	return cloneAttachments(e.attachments)
}

// cloneForExecute returns a shallow copy sharing session and deps store with per-call attachments and async runtime.
func (e *RunEnv) cloneForExecute(attachments []Attachment, async *asyncRuntime) *RunEnv {
	if e == nil {
		return &RunEnv{ //nolint:exhaustruct // nil env bootstrap
			store:       newRunEnvStore(),
			attachments: cloneAttachments(attachments),
			async:       async,
		}
	}
	return &RunEnv{
		Credentials: e.Credentials,
		StateStore:  e.StateStore,
		session:     e.session,
		store:       e.store,
		attachments: cloneAttachments(attachments),
		async:       async,
	}
}

// Put stores a session dependency. Prefer calling before tool execution; tools should read via [Require] or [Lookup].
// If env is nil, Put is a no-op (use [NewRunEnv] before wiring dependencies).
func Put[T any](env *RunEnv, key string, val T) {
	if env == nil || env.store == nil {
		return
	}
	env.store.mu.Lock()
	defer env.store.mu.Unlock()
	if env.store.deps == nil {
		env.store.deps = make(map[string]any)
	}
	env.store.deps[key] = val
}

// Require returns a dependency or a [ToolError] with [CodeDependencyMissing].
func Require[T any](env *RunEnv, key string) (T, error) {
	var zero T
	if env == nil || env.store == nil {
		return zero, NewDependencyMissingError(key)
	}
	env.store.mu.RLock()
	defer env.store.mu.RUnlock()
	v, ok := resolveTyped[T](env.store.deps, key)
	if !ok {
		return zero, NewDependencyMissingError(key)
	}
	return v, nil
}

// Lookup returns a dependency and whether it was present and non-nil.
func Lookup[T any](env *RunEnv, key string) (T, bool) {
	var zero T
	if env == nil || env.store == nil {
		return zero, false
	}
	env.store.mu.RLock()
	defer env.store.mu.RUnlock()
	return resolveTyped[T](env.store.deps, key)
}
