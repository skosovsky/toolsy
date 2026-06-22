package toolsy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"
)

// StateCodec encodes and decodes a typed session state value.
type StateCodec[T any] interface {
	Encode(T) ([]byte, error)
	Decode([]byte) (T, error)
}

// StateSlotPolicy controls import semantics for a registered state slot.
type StateSlotPolicy struct {
	Required bool
	Nullable bool
	SchemaID string
}

type stateSlotOptions struct {
	policy StateSlotPolicy
}

// StateSlotOption configures a registered state slot.
type StateSlotOption func(*stateSlotOptions)

// WithStateSlotRequired marks a state slot as required during snapshot import.
func WithStateSlotRequired() StateSlotOption {
	return func(o *stateSlotOptions) {
		o.policy.Required = true
	}
}

// WithStateSlotNullable allows explicit JSON null to be decoded by the slot codec.
func WithStateSlotNullable(nullable bool) StateSlotOption {
	return func(o *stateSlotOptions) {
		o.policy.Nullable = nullable
	}
}

// WithStateSlotSchemaID pins a host-owned schema version into session binding compatibility.
func WithStateSlotSchemaID(id string) StateSlotOption {
	return func(o *stateSlotOptions) {
		o.policy.SchemaID = id
	}
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
	statePolicy() StateSlotPolicy
	schemaFingerprint() string
}

type typedStateCodecEntry[T any] struct {
	codec  StateCodec[T]
	policy StateSlotPolicy
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

func (e typedStateCodecEntry[T]) statePolicy() StateSlotPolicy {
	return e.policy
}

func (e typedStateCodecEntry[T]) schemaFingerprint() string {
	valueType := reflect.TypeFor[T]()
	codecType := reflect.TypeOf(e.codec)
	return fmt.Sprintf(
		"typed:%s:%s:%s",
		reflectTypeFingerprint(valueType),
		reflectTypeFingerprint(codecType),
		e.policy.SchemaID,
	)
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
func RegisterStateCodec[T any](r *StateCodecRegistry, key string, codec StateCodec[T], opts ...StateSlotOption) error {
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
	r.codecs[key] = typedStateCodecEntry[T]{codec: codec, policy: stateSlotPolicy(opts)}
	return nil
}

// RegisterJSONCodec registers JSON marshal/unmarshal for type T at key.
func RegisterJSONCodec[T any](r *StateCodecRegistry, key string, opts ...StateSlotOption) error {
	return RegisterStateCodec(r, key, JSONStateCodec[T]{}, opts...)
}

// RegisterFromPrototype registers a JSON codec using the concrete type of prototype.
func (r *StateCodecRegistry) RegisterFromPrototype(key string, prototype any, opts ...StateSlotOption) error {
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
	return r.registerReflectCodec(key, typ, stateSlotPolicy(opts))
}

func (r *StateCodecRegistry) registerReflectCodec(key string, typ reflect.Type, policy StateSlotPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.codecs == nil {
		r.codecs = make(map[string]stateCodecEntry)
	}
	if _, exists := r.codecs[key]; exists {
		return fmt.Errorf("toolsy: state codec key %q already registered", key)
	}
	r.codecs[key] = reflectJSONCodec{typ: typ, policy: policy}
	return nil
}

type reflectJSONCodec struct {
	typ    reflect.Type
	policy StateSlotPolicy
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

func (c reflectJSONCodec) statePolicy() StateSlotPolicy {
	return c.policy
}

func (c reflectJSONCodec) schemaFingerprint() string {
	return fmt.Sprintf("reflect-json:%s:%s", reflectTypeFingerprint(c.typ), c.policy.SchemaID)
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

func (r *StateCodecRegistry) slotPolicies() map[string]StateSlotPolicy {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]StateSlotPolicy, len(r.codecs))
	for key, entry := range r.codecs {
		out[key] = entry.statePolicy()
	}
	return out
}

func stateSlotPolicy(opts []StateSlotOption) StateSlotPolicy {
	var cfg stateSlotOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.policy
}

func stateSchemaDigest(r *StateCodecRegistry) string {
	entries := r.slotSchemaEntries()
	if len(entries) == 0 {
		return ""
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	h := sha256.New()
	for _, key := range keys {
		entry := entries[key]
		policy := entry.statePolicy()
		fmt.Fprintf(
			h,
			"%s\x00%t\x00%t\x00%s\x00%s\x00",
			key,
			policy.Required,
			policy.Nullable,
			policy.SchemaID,
			entry.schemaFingerprint(),
		)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *StateCodecRegistry) slotSchemaEntries() map[string]stateCodecEntry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]stateCodecEntry, len(r.codecs))
	maps.Copy(out, r.codecs)
	return out
}

func reflectTypeFingerprint(typ reflect.Type) string {
	if typ == nil {
		return "<nil>"
	}
	if typ.PkgPath() == "" {
		return typ.String()
	}
	return typ.PkgPath() + "." + typ.String()
}

const sessionSnapshotVersion = 1

// SessionSnapshot is an opaque, versioned session state blob for persistence.
type SessionSnapshot struct { //nolint:recvcheck // MarshalJSON value receiver; UnmarshalJSON pointer receiver
	version int
	payload []byte
	binding SessionBinding
}

type sessionSnapshotWire struct {
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
	Binding SessionBinding  `json:"binding"`
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
	return SessionSnapshot{
		version: wire.Version,
		payload: append([]byte(nil), wire.Payload...),
		binding: cloneSessionBinding(wire.Binding),
	}, nil
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
		Binding: cloneSessionBinding(s.binding),
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
	s.binding = cloneSessionBinding(parsed.binding)
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

func (s SessionSnapshot) Binding() SessionBinding {
	return cloneSessionBinding(s.binding)
}
