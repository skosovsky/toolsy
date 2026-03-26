package testutil

import (
	"time"

	"github.com/skosovsky/toolsy"
)

const testRegistryTimeout = 30 * time.Second

// NewTestRegistry returns a Registry with long timeout and panic recovery enabled,
// suitable for tests.
func NewTestRegistry(tools ...toolsy.Tool) *toolsy.Registry {
	reg, err := toolsy.NewRegistryBuilder(
		toolsy.WithDefaultTimeout(testRegistryTimeout),
		toolsy.WithRecoverPanics(true),
	).Add(tools...).Build()
	if err != nil {
		panic(err)
	}
	return reg
}
