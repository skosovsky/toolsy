package toolsy

import (
	"encoding/json"
	"fmt"
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

// ExportSnapshot returns an opaque snapshot of in-memory session state.
// Dependencies, attachments, and StateStore are not included.
func (s *Session) ExportSnapshot() (SessionSnapshot, error) {
	if s == nil {
		return SessionSnapshot{}, NewValidationError("session is nil")
	}
	payload, err := s.encodeStatePayload()
	if err != nil {
		return SessionSnapshot{}, err
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	return SessionSnapshot{
		version: sessionSnapshotVersion,
		payload: payload,
	}, nil
}

// ImportSnapshot atomically replaces in-memory session state from snap.
// On error the previous state is unchanged.
func (s *Session) ImportSnapshot(snap SessionSnapshot) error {
	if s == nil {
		return NewValidationError("session is nil")
	}
	version, payload, err := snap.versionAndPayload()
	if err != nil {
		return err
	}
	if version != sessionSnapshotVersion {
		return fmt.Errorf("toolsy: unsupported session snapshot version %d", version)
	}
	newMap, err := s.decodeStatePayload(payload)
	if err != nil {
		return err
	}
	s.stateMu.Lock()
	s.state = newMap
	s.stateMu.Unlock()
	return nil
}

func (s *Session) encodeStatePayload() ([]byte, error) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if len(s.state) == 0 {
		return []byte("{}"), nil
	}
	wire := make(map[string]json.RawMessage, len(s.state))
	for k, v := range s.state {
		raw, err := s.encodeStateValue(k, v)
		if err != nil {
			return nil, fmt.Errorf("toolsy: export state key %q: %w", k, err)
		}
		wire[k] = raw
	}
	return json.Marshal(wire)
}

func (s *Session) encodeStateValue(key string, v any) (json.RawMessage, error) {
	if reg := s.opts.codecRegistry; reg != nil {
		if entry, ok := reg.lookup(key); ok {
			b, err := entry.encodeValue(v)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(b), nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func (s *Session) decodeStatePayload(payload []byte) (map[string]any, error) {
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, fmt.Errorf("toolsy: invalid session snapshot payload: %w", err)
	}
	if wire == nil {
		return make(map[string]any), nil
	}
	newMap := make(map[string]any, len(wire))
	for k, raw := range wire {
		val, err := s.decodeStateValue(k, raw)
		if err != nil {
			return nil, err
		}
		newMap[k] = val
	}
	return newMap, nil
}

func (s *Session) decodeStateValue(key string, raw json.RawMessage) (any, error) {
	if reg := s.opts.codecRegistry; reg != nil {
		if entry, ok := reg.lookup(key); ok {
			return entry.decodeBytes(raw)
		}
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("toolsy: failed to unmarshal state key %q: %w", key, err)
	}
	return generic, nil
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
