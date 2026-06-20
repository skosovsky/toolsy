package toolsy

import (
	"reflect"
	"sync"
)

// DepKeyBudget is the dependency map key for [BudgetTracker] used by [WithBudget].
const DepKeyBudget = "toolsy.budget"

// runEnvStore holds shared dependency maps; clones of [RunEnv] share the same store pointer.
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

// RunEnv is the per-call execution environment: credentials, persisted state store,
// keyed dependencies (deps), and optional binding to a [Session] for in-memory state.
type RunEnv struct {
	Credentials CredentialsProvider
	StateStore  StateStore

	session     *Session
	store       *runEnvStore
	attachments []Attachment
	callContext CallContext
	view        RegistryViewSnapshot
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

// NewRunEnv creates a run environment. When session is non-nil, [SetState] and [GetState]
// delegate to that session's in-memory state. session may be nil for DI-only usage.
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

// Attachments returns runtime attachments for the current call (cloned).
func (e *RunEnv) Attachments() []Attachment {
	if e == nil {
		return nil
	}
	return cloneAttachments(e.attachments)
}

// cloneForExecute returns a shallow copy sharing deps store and session pointer with per-call attachments.
func (e *RunEnv) cloneForExecute(attachments []Attachment, async *asyncRuntime, callContext ...CallContext) *RunEnv {
	bound := CallContext{
		Subject: nil,
		Scope:   nil,
		Metadata: CallMetadata{
			CallID:   "",
			ToolName: "",
			ViewID:   "",
			Tags:     nil,
		},
		Values: nil,
	}
	if len(callContext) > 0 {
		bound = cloneCallContext(callContext[0])
	} else if e != nil {
		bound = cloneCallContext(e.callContext)
	}
	if e == nil {
		return &RunEnv{ //nolint:exhaustruct // nil env bootstrap
			store:       newRunEnvStore(),
			attachments: cloneAttachments(attachments),
			callContext: bound,
			view: RegistryViewSnapshot{
				ID:                "",
				ToolNames:         nil,
				RequiredToolNames: nil,
				ManifestDigest:    "",
				Reason:            "",
				Owner:             "",
			},
			async: async,
		}
	}
	return &RunEnv{
		session:     e.session,
		Credentials: e.Credentials,
		StateStore:  e.StateStore,
		store:       e.store,
		attachments: cloneAttachments(attachments),
		callContext: bound,
		view:        cloneRegistryViewSnapshot(e.view),
		async:       async,
	}
}

// RegistryViewSnapshot returns the capability view bound to the current execution, if any.
func (e *RunEnv) RegistryViewSnapshot() RegistryViewSnapshot {
	if e == nil {
		return RegistryViewSnapshot{}
	}
	return cloneRegistryViewSnapshot(e.view)
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

// SetState stores mutable session-scoped data on the bound [Session].
// If env or env.session is nil, SetState is a no-op.
func SetState[T any](env *RunEnv, key string, val T) {
	if env == nil || env.session == nil {
		return
	}
	SetSessionState(env.session, key, val)
}

// GetState returns session-scoped data from the bound [Session] when present and non-nil.
func GetState[T any](env *RunEnv, key string) (T, bool) {
	if env == nil || env.session == nil {
		var zero T
		return zero, false
	}
	return GetSessionState[T](env.session, key)
}

func resolveTyped[T any](store map[string]any, key string) (T, bool) {
	var zero T
	if store == nil {
		return zero, false
	}
	raw, ok := store[key]
	if !ok {
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		return zero, false
	}
	if isNilValue(typed) {
		return zero, false
	}
	return typed, true
}

func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
