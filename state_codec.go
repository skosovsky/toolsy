package toolsy

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// StateCodec encodes and decodes a typed session state value.
type StateCodec[T any] interface {
	Encode(T) ([]byte, error)
	Decode([]byte) (T, error)
}

// JSONStateCodec is a [StateCodec] using encoding/json.
type JSONStateCodec[T any] struct{}

func (JSONStateCodec[T]) Encode(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONStateCodec[T]) Decode(data []byte) (T, error) {
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

type stateCodecEntry interface {
	encodeValue(v any) ([]byte, error)
	decodeBytes(data []byte) (any, error)
}

type typedStateCodecEntry[T any] struct {
	codec StateCodec[T]
}

func (e typedStateCodecEntry[T]) encodeValue(v any) ([]byte, error) {
	typed, ok := v.(T)
	if !ok {
		return nil, NewSnapshotHydrationError(
			"state value type mismatch",
			errors.New("toolsy: state value type mismatch for key"),
		)
	}
	return e.codec.Encode(typed)
}

func (e typedStateCodecEntry[T]) decodeBytes(data []byte) (any, error) {
	return e.codec.Decode(data)
}

// StateCodecRegistry maps session state keys to codecs.
type StateCodecRegistry struct {
	mu     sync.RWMutex
	codecs map[string]stateCodecEntry
}

// NewStateCodecRegistry creates an empty codec registry.
func NewStateCodecRegistry() *StateCodecRegistry {
	return &StateCodecRegistry{ //nolint:exhaustruct // mu zero value
		codecs: make(map[string]stateCodecEntry),
	}
}

// RegisterStateCodec associates key with codec for type T.
func RegisterStateCodec[T any](r *StateCodecRegistry, key string, codec StateCodec[T]) error {
	if r == nil {
		return errors.New("toolsy: nil StateCodecRegistry")
	}
	if key == "" {
		return errors.New("toolsy: state codec key is required")
	}
	if codec == nil {
		return errors.New("toolsy: state codec must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.codecs == nil {
		r.codecs = make(map[string]stateCodecEntry)
	}
	if _, exists := r.codecs[key]; exists {
		return fmt.Errorf("toolsy: state codec key %q already registered", key)
	}
	r.codecs[key] = typedStateCodecEntry[T]{codec: codec}
	return nil
}

// RegisterJSONCodec registers JSON marshal/unmarshal for type T at key.
func RegisterJSONCodec[T any](r *StateCodecRegistry, key string) error {
	return RegisterStateCodec(r, key, JSONStateCodec[T]{})
}

// RegisterFromPrototype registers a JSON codec using the concrete type of prototype.
func (r *StateCodecRegistry) RegisterFromPrototype(key string, prototype any) error {
	if r == nil {
		return errors.New("toolsy: nil StateCodecRegistry")
	}
	if key == "" {
		return errors.New("toolsy: state codec key is required")
	}
	if prototype == nil {
		return errors.New("toolsy: state codec prototype is required")
	}
	typ := reflect.TypeOf(prototype)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return r.registerReflectCodec(key, typ)
}

func (r *StateCodecRegistry) registerReflectCodec(key string, typ reflect.Type) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.codecs == nil {
		r.codecs = make(map[string]stateCodecEntry)
	}
	if _, exists := r.codecs[key]; exists {
		return fmt.Errorf("toolsy: state codec key %q already registered", key)
	}
	r.codecs[key] = reflectJSONCodec{typ: typ}
	return nil
}

type reflectJSONCodec struct {
	typ reflect.Type
}

func (c reflectJSONCodec) encodeValue(v any) ([]byte, error) {
	if v == nil {
		return json.Marshal(v)
	}
	typ := reflect.TypeOf(v)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ != c.typ {
		return nil, NewSnapshotHydrationError(
			"state value type mismatch",
			errors.New("toolsy: state value type mismatch for key"),
		)
	}
	return json.Marshal(v)
}

func (c reflectJSONCodec) decodeBytes(data []byte) (any, error) {
	ptr := reflect.New(c.typ).Interface()
	if err := json.Unmarshal(data, ptr); err != nil {
		return nil, err
	}
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}

func (r *StateCodecRegistry) lookup(key string) (stateCodecEntry, bool) {
	if r == nil || key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.codecs[key]
	return entry, ok
}

const sessionSnapshotVersion = 1

// SessionSnapshot is an opaque, versioned session state blob for persistence.
type SessionSnapshot struct { //nolint:recvcheck // MarshalJSON value receiver; UnmarshalJSON pointer receiver
	version int
	payload []byte
}

type sessionSnapshotWire struct {
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

// NewSessionSnapshotFromJSON parses snapshot bytes from storage.
func NewSessionSnapshotFromJSON(data []byte) (SessionSnapshot, error) {
	var wire sessionSnapshotWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return SessionSnapshot{}, NewSnapshotHydrationError(
			"invalid session snapshot",
			fmt.Errorf("toolsy: invalid session snapshot: %w", err),
		)
	}
	if wire.Version <= 0 {
		return SessionSnapshot{}, NewSnapshotHydrationError(
			"session snapshot version is required",
			errors.New("toolsy: session snapshot version is required"),
		)
	}
	if len(wire.Payload) == 0 {
		return SessionSnapshot{}, NewSnapshotHydrationError(
			"session snapshot payload is required",
			errors.New("toolsy: session snapshot payload is required"),
		)
	}
	return SessionSnapshot{version: wire.Version, payload: append([]byte(nil), wire.Payload...)}, nil
}

// MarshalJSON serializes the snapshot for persistence.
func (s SessionSnapshot) MarshalJSON() ([]byte, error) {
	if s.version <= 0 || len(s.payload) == 0 {
		return nil, NewSnapshotHydrationError(
			"empty session snapshot",
			errors.New("toolsy: empty session snapshot"),
		)
	}
	return json.Marshal(sessionSnapshotWire{
		Version: s.version,
		Payload: json.RawMessage(s.payload),
	})
}

// UnmarshalJSON restores snapshot from JSON storage.
func (s *SessionSnapshot) UnmarshalJSON(data []byte) error {
	parsed, err := NewSessionSnapshotFromJSON(data)
	if err != nil {
		return err
	}
	s.version = parsed.version
	s.payload = parsed.payload
	return nil
}

func (s SessionSnapshot) versionAndPayload() (int, []byte, error) {
	if s.version <= 0 || len(s.payload) == 0 {
		return 0, nil, NewSnapshotHydrationError(
			"empty session snapshot",
			errors.New("toolsy: empty session snapshot"),
		)
	}
	return s.version, append([]byte(nil), s.payload...), nil
}
