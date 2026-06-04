package toolsy

import (
	"fmt"
	"reflect"
	"sync"
)

// StateTypeRegistry maps string keys to Go types for [Session.Import] deserialization.
// Pass an instance via [WithStateTypeRegistry]; toolsy has no package-level registry.
type StateTypeRegistry struct {
	mu    sync.RWMutex
	types map[string]reflect.Type
}

// NewStateTypeRegistry creates an empty state type registry.
func NewStateTypeRegistry() *StateTypeRegistry {
	return &StateTypeRegistry{ //nolint:exhaustruct // mu is zero value
		types: make(map[string]reflect.Type),
	}
}

// Register associates a state key with the Go type of prototype.
// Prefer a non-pointer value (e.g. MyStruct{}); pointer prototypes are normalized to their element type.
// Panics if key is empty, prototype is nil, or the key is already registered with a different type.
func (r *StateTypeRegistry) Register(key string, prototype any) {
	if key == "" {
		panic("toolsy: StateTypeRegistry.Register key must not be empty")
	}
	if prototype == nil {
		panic("toolsy: StateTypeRegistry.Register prototype must not be nil")
	}
	typ := reflect.TypeOf(prototype)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.types == nil {
		r.types = make(map[string]reflect.Type)
	}
	if existing, exists := r.types[key]; exists && existing != typ {
		panic(fmt.Sprintf("toolsy: state type for key %q already registered with different type", key))
	}
	r.types[key] = typ
}

func (r *StateTypeRegistry) lookup(key string) (reflect.Type, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	typ, ok := r.types[key]
	return typ, ok
}
