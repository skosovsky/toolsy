package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestHostStateTimezoneExample(t *testing.T) {
	// Arrange: run supplies an immutable local StateStore explicitly.
	// Act.
	err := run(t.Context())
	// Assert: bounded host timezone resolution and both calendar/duration calls succeed.
	require.NoError(t, err)
}

type failedStore struct{ failure error }

func (s failedStore) Load(context.Context, string) ([]byte, error) { return nil, s.failure }
func (s failedStore) Save(context.Context, string, []byte) error   { return s.failure }

func TestHostTimezoneStateFailures(t *testing.T) {
	// Arrange.
	for _, env := range []*toolsy.RunEnv{nil, toolsy.NewRunEnv(nil)} {
		// Act.
		loc, err := loadLocation(t.Context(), env)
		// Assert.
		require.ErrorContains(t, err, "StateStore is required")
		require.Nil(t, loc)
	}
	for _, fixture := range []struct{ zone, reason string }{
		{strings.Repeat("x", 65), "exceeds 64 bytes"},
		{strings.Repeat("x", 64), "unsupported timezone"},
		{"", "unsupported timezone"},
	} {
		env := toolsy.NewRunEnv(nil, toolsy.WithStateStore(fixtureStore{zone: fixture.zone}))
		// Act.
		loc, err := loadLocation(t.Context(), env)
		// Assert: exact 64 bytes pass the byte gate, but still need host zone selection.
		require.ErrorContains(t, err, fixture.reason)
		require.Nil(t, loc)
	}
	failure := errors.New("store unavailable")
	env := toolsy.NewRunEnv(nil, toolsy.WithStateStore(failedStore{failure: failure}))
	loc, err := loadLocation(t.Context(), env)
	require.ErrorIs(t, err, failure)
	require.Nil(t, loc)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	env = toolsy.NewRunEnv(nil, toolsy.WithStateStore(fixtureStore{zone: "UTC"}))
	loc, err = loadLocation(ctx, env)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, loc)
}
