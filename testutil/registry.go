package testutil

import (
	"time"

	"github.com/skosovsky/toolsy"
)

const testRegistryTimeout = 30 * time.Second

// NewTestRegistry returns a Registry with long timeout and panic recovery enabled,
// suitable for tests.
func NewTestRegistry(tools ...toolsy.Tool) *toolsy.Registry {
	reg := toolsy.NewRegistry(
		toolsy.WithDefaultTimeout(testRegistryTimeout),
		toolsy.WithRecoverPanics(true),
	)
	for _, t := range tools {
		reg.Register(t)
	}
	return reg
}
