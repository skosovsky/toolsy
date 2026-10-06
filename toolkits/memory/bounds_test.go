package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestScratchpadConfiguration(t *testing.T) {
	// Arrange.
	for _, opt := range []Option{nil, WithMaxFacts(-1), WithMaxKeyBytes(-1), WithMaxValueBytes(-1), WithMaxStoreBytes(-1), WithMaxOutputBytes(-1)} {
		// Act.
		pad, err := NewScratchpad(opt)
		// Assert.
		require.Error(t, err)
		require.Nil(t, pad)
	}
	pad := mustScratchpad(t, WithMaxFacts(0), WithMaxValueBytes(0))
	_, err := pad.AsTools()
	require.NoError(t, err)
	_, err = mustScratchpad(t, WithMaxOutputBytes(1)).AsTools()
	require.Error(t, err)
}

func mustScratchpad(t *testing.T, opts ...Option) *Scratchpad {
	t.Helper()
	pad, err := NewScratchpad(opts...)
	require.NoError(t, err)
	return pad
}

func TestScratchpadRejectedPinDoesNotMutateStore(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		opt        Option
		key, value string
	}{
		{"value", WithMaxValueBytes(4), "key", "longvalue"},
		{"key", WithMaxKeyBytes(2), "longkey", "v"},
		{"escapedStore", WithMaxStoreBytes(30), "key", strings.Repeat("<", 10)},
		{"emptyKey", WithMaxFacts(1), "", "v"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			// Arrange.
			pad := mustScratchpad(t, fixture.opt)
			store := newMemStateStore()
			run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
			// Act.
			_, err := pad.pinHandler(context.Background(), run, pinArgs{Key: fixture.key, Value: fixture.value})
			// Assert.
			require.Error(t, err)
			raw, err := store.Load(context.Background(), factsStateKey)
			require.NoError(t, err)
			require.Empty(t, raw)
		})
	}
}

func TestScratchpadExistingStateFailsClosed(t *testing.T) {
	for _, raw := range []string{`null`, `{"a":null}`, `{"a":"first","a":"second"}`, `{"toolongkey":"v"}`, `{"a":"too long"}`, `{"a":"v","b":"v"}`, strings.Repeat(" ", 101), string([]byte{'{', '"', 'a', '"', ':', '"', 255, '"', '}'})} {
		// Arrange.
		pad := mustScratchpad(t, WithMaxFacts(1), WithMaxKeyBytes(2), WithMaxValueBytes(2), WithMaxStoreBytes(100))
		store := newMemStateStore()
		require.NoError(t, store.Save(context.Background(), factsStateKey, []byte(raw)))
		run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
		// Act.
		_, readErr := pad.readHandler(context.Background(), run, struct{}{})
		_, pinErr := pad.pinHandler(context.Background(), run, pinArgs{Key: "a", Value: "v"})
		_, unpinErr := pad.unpinHandler(context.Background(), run, unpinArgs{Key: "a"})
		// Assert.
		require.Error(t, readErr, raw)
		require.Error(t, pinErr, raw)
		require.Error(t, unpinErr, raw)
		actual, err := store.Load(context.Background(), factsStateKey)
		require.NoError(t, err)
		require.Equal(t, []byte(raw), actual)
	}
}

func TestScratchpadWireBytesAfterEscaping(t *testing.T) {
	// Arrange.
	pad := mustScratchpad(t, WithMaxOutputBytes(64))
	store := newMemStateStore()
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	value := strings.Repeat("<", 20)
	_, err := pad.pinHandler(context.Background(), run, pinArgs{Key: "a", Value: value})
	require.NoError(t, err)
	// Act.
	_, err = pad.readHandler(context.Background(), run, struct{}{})
	// Assert.
	require.Error(t, err)
	raw, err := store.Load(context.Background(), factsStateKey)
	require.NoError(t, err)
	var facts map[string]string
	require.NoError(t, json.Unmarshal(raw, &facts))
	require.Equal(t, value, facts["a"])
}

func TestMutationWirePreflight(t *testing.T) {
	// Arrange.
	pad := mustScratchpad(t, WithMaxOutputBytes(1))
	store := newMemStateStore()
	require.NoError(t, store.Save(context.Background(), factsStateKey, []byte(`{"a":"v"}`)))
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	// Act.
	_, pinErr := pad.pinHandler(context.Background(), run, pinArgs{Key: "a", Value: "changed"})
	_, unpinErr := pad.unpinHandler(context.Background(), run, unpinArgs{Key: "a"})
	_, ignoredErr := pad.unpinHandler(context.Background(), run, unpinArgs{Key: "absent"})
	// Assert.
	require.Error(t, pinErr)
	require.Error(t, unpinErr)
	require.Error(t, ignoredErr)
	raw, err := store.Load(context.Background(), factsStateKey)
	require.NoError(t, err)
	require.JSONEq(t, `{"a":"v"}`, string(raw))
}

func TestFiniteDefaults(t *testing.T) {
	// Arrange.
	pad := mustScratchpad(t)
	store := newMemStateStore()
	run := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	facts := make(map[string]string)
	for i := range 128 {
		facts[strings.Repeat("k", i+1)] = "v"
	}
	raw, err := json.Marshal(facts)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), factsStateKey, raw))
	// Act.
	_, countErr := pad.pinHandler(context.Background(), run, pinArgs{Key: "new", Value: "v"})
	_, valueErr := pad.pinHandler(context.Background(), run, pinArgs{Key: "k", Value: strings.Repeat("界", 1366)})
	// Assert.
	require.Error(t, countErr)
	require.Error(t, valueErr)
	actual, err := store.Load(context.Background(), factsStateKey)
	require.NoError(t, err)
	require.Equal(t, raw, actual)
}
