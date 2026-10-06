package memory_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/memory"
)

type r18BlockedStore struct {
	entered, release chan struct{}
	loads, saves     atomic.Int32
}

func (s *r18BlockedStore) Load(ctx context.Context, _ string) ([]byte, error) {
	if s.loads.Add(1) == 1 {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, nil
}
func (s *r18BlockedStore) Save(context.Context, string, []byte) error { s.saves.Add(1); return nil }

func TestR18PublicCanceledWaiterNeverCallsStore(t *testing.T) {
	for i, args := range []string{`{"key":"k","value":"v"}`, `{}`, `{"key":"k"}`} {
		for _, manual := range []bool{false, true} {
			t.Run(
				fmt.Sprintf("tool%d/manual%v", i, manual),
				func(t *testing.T) { r18CanceledWaitCase(t, i, args, manual) },
			)
		}
	}
}
func r18CanceledWaitCase(t *testing.T, index int, args string, manual bool) {
	t.Helper()
	// Arrange: active read owns admission while host Load cooperatively blocks.
	tools, err := memory.NewScratchpad().AsTools()
	require.NoError(t, err)
	store := &r18BlockedStore{entered: make(chan struct{}), release: make(chan struct{})}
	env := toolsy.NewRunEnv(nil, toolsy.WithStateStore(store))
	first := make(chan error, 1)
	go func() {
		first <- tools[1].Execute(t.Context(), env, toolsy.ToolInput{ArgsJSON: []byte(`{}`)}, func(toolsy.Chunk) error { return nil })
	}()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("first did not reach Load")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	if manual {
		cancel()
		ctx, cancel = context.WithCancel(t.Context())
		time.AfterFunc(10*time.Millisecond, cancel)
	}
	defer cancel()
	waiter := make(chan error, 1)
	go func() {
		waiter <- tools[index].Execute(ctx, env, toolsy.ToolInput{ArgsJSON: []byte(args)}, func(toolsy.Chunk) error { return nil })
	}()
	// Act.
	var waitErr error
	returned := false
	select {
	case waitErr = <-waiter:
		returned = true
	case <-time.After(200 * time.Millisecond):
	}
	loads, saves := store.loads.Load(), store.saves.Load()
	close(store.release)
	if !returned {
		waitErr = r18Await(t, waiter)
	}
	require.NoError(t, r18Await(t, first))
	// Assert after joining fixture: canceled admission cannot invoke store.
	require.True(t, returned, "waiter ignored cancellation until provider release")
	require.ErrorIs(t, waitErr, ctx.Err())
	require.EqualValues(t, 1, loads)
	require.Zero(t, saves)
	require.EqualValues(t, 1, store.loads.Load())
	require.Zero(t, store.saves.Load())
}
func r18Await(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("fixture failed to join")
		return nil
	}
}
