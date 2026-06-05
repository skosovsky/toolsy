package toolsy

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
)

// GetSessionState returns in-memory session state for key when present and non-nil.
func GetSessionState[T any](s *Session, key string) (T, bool) {
	var zero T
	if s == nil || key == "" {
		return zero, false
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return resolveTyped[T](s.state, key)
}

// SetSessionState stores in-memory session state for key.
func SetSessionState[T any](s *Session, key string, val T) {
	if s == nil || key == "" {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state == nil {
		s.state = make(map[string]any)
	}
	s.state[key] = val
}

// Export returns a shallow copy of the session in-memory state map.
// Dependencies, attachments, and StateStore are not included.
// Do not mutate the returned map in place; treat it as a snapshot.
func (s *Session) Export() map[string]any {
	if s == nil {
		return map[string]any{}
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if len(s.state) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(s.state))
	maps.Copy(out, s.state)
	return out
}

// Import atomically replaces in-memory session state from data.
// Import(nil) clears all keys. On error the previous state is unchanged.
func (s *Session) Import(data map[string]any) error {
	if s == nil {
		return NewValidationError("session is nil")
	}
	if data == nil {
		s.stateMu.Lock()
		s.state = make(map[string]any)
		s.stateMu.Unlock()
		return nil
	}
	newMap, err := s.buildImportedState(data)
	if err != nil {
		return err
	}
	s.stateMu.Lock()
	s.state = newMap
	s.stateMu.Unlock()
	return nil
}

func (s *Session) buildImportedState(data map[string]any) (map[string]any, error) {
	reg := s.opts.typeRegistry
	newMap := make(map[string]any, len(data))
	for k, v := range data {
		if reg != nil {
			if typ, ok := reg.lookup(k); ok {
				typed, err := importRegisteredValue(k, v, typ)
				if err != nil {
					return nil, err
				}
				newMap[k] = typed
				continue
			}
		}
		newMap[k] = v
	}
	return newMap, nil
}

func importRegisteredValue(key string, v any, typ reflect.Type) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal registered state key %q: %w", key, err)
	}
	ptr := reflect.New(typ).Interface()
	if err := json.Unmarshal(b, ptr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal registered state key %q: %w", key, err)
	}
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}

// ValidateRunEnvSession reports when env is not bound to session.
func ValidateRunEnvSession(session *Session, env *RunEnv) error {
	if session == nil {
		return NewValidationError("session is nil")
	}
	if env == nil {
		return NewValidationError("run environment is nil")
	}
	if env.session != session {
		return NewValidationError("run environment is not bound to this session")
	}
	return nil
}
