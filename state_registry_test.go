package toolsy

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateTypeRegistry_Register_Lookup(t *testing.T) {
	reg := NewStateTypeRegistry()
	type payload struct {
		N int `json:"n"`
	}
	reg.Register("p", payload{})

	typ, ok := reg.lookup("p")
	require.True(t, ok)
	assert.Equal(t, reflect.TypeFor[payload](), typ)
}

func TestStateTypeRegistry_Register_Idempotent(t *testing.T) {
	reg := NewStateTypeRegistry()
	reg.Register("k", struct{ A int }{})
	assert.NotPanics(t, func() { reg.Register("k", struct{ A int }{}) })
}

func TestStateTypeRegistry_Register_ConflictPanics(t *testing.T) {
	reg := NewStateTypeRegistry()
	reg.Register("k", struct{ A int }{})
	assert.Panics(t, func() { reg.Register("k", struct{ B string }{}) })
}

func TestStateTypeRegistry_Lookup_NilRegistry(t *testing.T) {
	var reg *StateTypeRegistry
	_, ok := reg.lookup("k")
	assert.False(t, ok)
}

func TestStateTypeRegistry_Register_NilPrototypePanics(t *testing.T) {
	reg := NewStateTypeRegistry()
	assert.Panics(t, func() { reg.Register("k", nil) })
}

func TestStateTypeRegistry_Register_EmptyKeyPanics(t *testing.T) {
	reg := NewStateTypeRegistry()
	assert.Panics(t, func() { reg.Register("", struct{}{}) })
}

func TestStateTypeRegistry_Register_PointerPrototypeNormalized(t *testing.T) {
	reg := NewStateTypeRegistry()
	type payload struct{ N int }
	reg.Register("p", &payload{})

	typ, ok := reg.lookup("p")
	require.True(t, ok)
	assert.Equal(t, reflect.TypeFor[payload](), typ)
}
