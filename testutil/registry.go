package testutil

import (
	"time"

	"github.com/skosovsky/toolsy"
)

// NewTestRegistry returns a Registry with long timeout and panic recovery enabled,
// suitable for tests.
func NewTestRegistry(tools ...toolsy.Tool) *toolsy.Registry {
	reg := toolsy.NewRegistry(
		toolsy.WithDefaultTimeout(30*time.Second),
		toolsy.WithRecoverPanics(true),
	)
	for _, t := range tools {
		reg.Register(t)
	}
	return reg
}
