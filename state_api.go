package toolsy

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// GetSessionState returns in-memory session state when present and non-nil.
func GetSessionState[T any](s *Session, key string) (T, bool) {
	var zero T
	if s == nil {
		return zero, false
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return resolveTyped[T](s.state, key)
}

// SetSessionState stores in-memory session state shared across tool calls in this session.
func SetSessionState[T any](s *Session, key string, val T) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state == nil {
		s.state = make(map[string]any)
	}
	s.state[key] = val
}

// GetState returns session-scoped data via the bound [Session] when env is wired.
// If env or env.session is nil, returns zero value and false.
func GetState[T any](env *RunEnv, key string) (T, bool) {
	var zero T
	if env == nil || env.session == nil {
		return zero, false
	}
	return GetSessionState[T](env.session, key)
}

// SetState stores mutable session-scoped data on the bound [Session].
// If env or env.session is nil, SetState is a no-op.
func SetState[T any](env *RunEnv, key string, val T) {
	if env == nil || env.session == nil {
		return
	}
	SetSessionState(env.session, key, val)
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

func importStateValue(registry *StateTypeRegistry, key string, v any) (any, error) {
	typ, ok := registry.lookup(key)
	if !ok {
		return v, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("session import: marshal key %q: %v", key, err))
	}
	ptr := reflect.New(typ).Interface()
	if err := json.Unmarshal(b, ptr); err != nil {
		return nil, NewValidationError(fmt.Sprintf("session import: unmarshal key %q: %v", key, err))
	}
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}
