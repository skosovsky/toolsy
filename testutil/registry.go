package testutil

import (
	"github.com/skosovsky/toolsy"
)

// NewTestRegistry returns a Registry with panic recovery enabled, suitable for tests.
func NewTestRegistry(tools ...toolsy.Tool) *toolsy.Registry {
	reg, err := toolsy.NewRegistryBuilder(
		toolsy.WithRecoverPanics(true),
	).Add(tools...).Build()
	if err != nil {
		panic(err)
	}
	return reg
}
