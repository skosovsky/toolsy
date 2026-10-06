package memory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type gateStore struct {
	load  func(context.Context) ([]byte, error)
	saves int
}

func (s *gateStore) Load(ctx context.Context, _ string) ([]byte, error) { return s.load(ctx) }
func (s *gateStore) Save(context.Context, string, []byte) error         { s.saves++; return nil }
func TestScratchpadCancellationAfterLoadPreventsSave(t *testing.T) {
	// Arrange: host returns data but cancels during its callback.
	pad := mustScratchpad(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store := &gateStore{load: func(context.Context) ([]byte, error) { cancel(); return []byte(`{"k":"v"}`), nil }}
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	// Act.
	_, err := pad.pinHandler(ctx, run, pinArgs{Key: "k", Value: "changed"})
	// Assert: canceled update never calls Save and admission is released for another call.
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, store.saves)
	store.load = func(context.Context) ([]byte, error) { return nil, nil }
	_, err = pad.pinHandler(t.Context(), run, pinArgs{Key: "next", Value: "v"})
	require.NoError(t, err)
	require.Equal(t, 1, store.saves)
}
func TestScratchpadGateReleasesAfterProviderErrors(t *testing.T) {
	// Arrange.
	pad := mustScratchpad(t)
	cause := errors.New("load failed")
	store := &gateStore{load: func(context.Context) ([]byte, error) { return nil, cause }}
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	// Act.
	_, err := pad.readHandler(t.Context(), run, struct{}{})
	// Assert.
	require.ErrorIs(t, err, cause)
	store.load = func(context.Context) ([]byte, error) { return nil, nil }
	_, err = pad.pinHandler(t.Context(), run, pinArgs{Key: "k", Value: "v"})
	require.NoError(t, err)
	require.Equal(t, 1, store.saves)
}
func TestScratchpadStructuredEscapingAndWireBoundary(t *testing.T) {
	// Arrange.
	facts := map[string]string{"key=\n\"<branch>": "value=\n\r\x00\\Привет"}
	raw, err := json.Marshal(facts)
	require.NoError(t, err)
	store := newMemStateStore()
	require.NoError(t, store.Save(t.Context(), factsStateKey, raw))
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	expected, err := json.Marshal(readResult{Facts: facts})
	require.NoError(t, err)
	for _, n := range []int{len(expected), len(expected) - 1} {
		pad := mustScratchpad(t, WithMaxOutputBytes(n))
		// Act.
		got, err := pad.readHandler(t.Context(), run, struct{}{})
		// Assert.
		if n == len(expected) {
			require.NoError(t, err)
			require.Equal(t, facts, got.Facts)
		} else {
			require.ErrorIs(t, err, toolsy.ErrValidation)
		}
	}
}
