package toolsy

import (
	"errors"
	"reflect"
	"sync"
)

// StateTypeRegistry maps state keys to concrete Go types for safe deserialization after JSON roundtrips.
type StateTypeRegistry struct {
	mu    sync.RWMutex
	types map[string]reflect.Type
}

// NewStateTypeRegistry creates an empty type registry.
func NewStateTypeRegistry() *StateTypeRegistry {
	return &StateTypeRegistry{ //nolint:exhaustruct // mu zero value
		types: make(map[string]reflect.Type),
	}
}

// Register associates key with the concrete type of prototype (e.g. MyStruct{} or &MyStruct{}).
// Returns an error when r, key, or prototype is invalid.
func (r *StateTypeRegistry) Register(key string, prototype any) error {
	if r == nil {
		return errors.New("toolsy: nil StateTypeRegistry")
	}
	if key == "" {
		return errors.New("toolsy: state type registry key is required")
	}
	if prototype == nil {
		return errors.New("toolsy: state type registry prototype is required")
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
	r.types[key] = typ
	return nil
}

func (r *StateTypeRegistry) lookup(key string) (reflect.Type, bool) {
	if r == nil || key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	typ, ok := r.types[key]
	return typ, ok
}
