package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sessionConcurrencyRegistry(t *testing.T, value string) *Registry {
	t.Helper()
	tool, err := NewTool[struct{}, string](
		"value",
		"Return a value",
		func(context.Context, *RunEnv, struct{}) (string, error) { return value, nil },
	)
	require.NoError(t, err)
	registry, err := NewRegistry(tool)
	require.NoError(t, err)
	return registry
}

func TestConcurrentSessionRebindExecutionAndCheckpoint(t *testing.T) {
	// Arrange: two independently built registries have the same execution contract.
	first, second := sessionConcurrencyRegistry(t, "first"), sessionConcurrencyRegistry(t, "second")
	session, err := NewSession(first)
	require.NoError(t, err)
	if mutationErr := SetSessionState(session, "value", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	initial := session.Binding()
	failures := make(chan error, 1000)
	var workers sync.WaitGroup
	// Act: independent writers, readers and executions overlap.
	for worker := range 6 {
		workers.Go(func() { runSessionConcurrencyWorker(t.Context(), session, first, second, worker, failures) })
	}
	workers.Wait()
	close(failures)
	// Assert.
	for err := range failures {
		t.Error(err)
	}
	require.Equal(t, initial, session.Binding())
}

func runSessionConcurrencyWorker(
	ctx context.Context,
	session *Session,
	first, second *Registry,
	worker int,
	failures chan<- error,
) {
	for iteration := range 100 {
		var err error
		switch worker % 3 {
		case 0:
			registry := first
			if iteration%2 == 0 {
				registry = second
			}
			err = session.Rebind(registry)
		case 1:
			err = probeSessionCheckpointClone(session)
		case 2:
			err = probeSessionExecution(ctx, session, iteration)
		}
		if err != nil {
			failures <- err
		}
	}
}

func probeSessionCheckpointClone(session *Session) error {
	checkpoint, err := session.ExportCheckpoint()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(checkpoint.Binding, checkpoint.Snapshot.Binding()) {
		return errors.New("checkpoint binding differs")
	}
	if err = session.ImportSnapshot(checkpoint.Snapshot); err != nil {
		return err
	}
	before := session.Binding()
	copied := session.Binding()
	copied.ToolNames[0] = "caller mutation"
	copied.View.ToolNames = append(copied.View.ToolNames, "caller mutation")
	if !reflect.DeepEqual(before, session.Binding()) {
		return errors.New("binding not independently cloned")
	}
	return nil
}

func probeSessionExecution(ctx context.Context, session *Session, iteration int) error {
	call := ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	if iteration%2 == 0 {
		return session.Execute(ctx, call, func(Chunk) error { return nil })
	}
	outcome, err := session.RunCall(ctx, call)
	if err != nil {
		return err
	}
	decoded, err := DecodeOutcomeAs[string](outcome)
	if err != nil {
		return err
	}
	if *decoded != "first" && *decoded != "second" {
		return errors.New("unexpected registry output")
	}
	return nil
}

type rebindManifestTool struct {
	Tool

	armed atomic.Bool
	after func()
}

func (t *rebindManifestTool) Manifest() ToolManifest {
	manifest := t.Tool.Manifest()
	if t.armed.CompareAndSwap(true, false) {
		t.after()
	}
	return manifest
}

func TestRunCallCapturesRegistryBeforeManifestCallback(t *testing.T) {
	// Arrange: compatible rebind during the manifest callback must affect the next call only.
	target := sessionConcurrencyRegistry(t, "second")
	source := sessionConcurrencyRegistry(t, "first")
	tool, _ := source.GetTool("value")
	decorated := &rebindManifestTool{Tool: tool}
	registry, err := NewRegistry(decorated)
	require.NoError(t, err)
	session, err := NewSession(registry)
	require.NoError(t, err)
	decorated.after = func() { require.NoError(t, session.Rebind(target)) }
	decorated.armed.Store(true)
	// Act.
	first, err := session.RunCall(t.Context(), ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}})
	require.NoError(t, err)
	second, err := session.RunCall(t.Context(), ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}})
	require.NoError(t, err)
	// Assert.
	firstValue, err := DecodeOutcomeAs[string](first)
	require.NoError(t, err)
	secondValue, err := DecodeOutcomeAs[string](second)
	require.NoError(t, err)
	require.Equal(t, "first", *firstValue)
	require.Equal(t, "second", *secondValue)
}

type reentryStateCodec struct{ callback func() }

func (c reentryStateCodec) Encode(value int) ([]byte, error) {
	c.callback()
	return json.Marshal(value)
}
func (c reentryStateCodec) Decode(raw []byte) (int, error) {
	c.callback()
	var value int
	err := json.Unmarshal(raw, &value)
	return value, err
}

func TestSnapshotCallbacksReenterWithoutLocks(t *testing.T) {
	// Arrange.
	registry := sessionConcurrencyRegistry(t, "first")
	var session *Session
	codecs := NewStateCodecRegistry()
	callback := func() {
		if mutationErr := SetSessionState(session, "callback", 1); mutationErr != nil {
			t.Error(mutationErr)
		}
		_ = session.Binding()
		if err := session.Rebind(registry); err != nil {
			t.Error(err)
		}
	}
	require.NoError(t, RegisterStateCodec[int](codecs, "value", reentryStateCodec{callback: callback}))
	var err error
	session, err = NewSession(registry, WithStateCodecRegistry(codecs))
	require.NoError(t, err)
	if mutationErr := SetSessionState(session, "value", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	finished := make(chan error, 1)
	// Act: export and import both invoke the host callback.
	go func() {
		checkpoint, exportErr := session.ExportCheckpoint()
		if exportErr != nil {
			finished <- exportErr
			return
		}
		finished <- session.ImportSnapshot(checkpoint.Snapshot)
	}()
	// Assert.
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot callback blocked under a lock")
	}
	value, ok := GetSessionState[int](session, "value")
	require.True(t, ok)
	require.Equal(t, 7, value)
}

func TestIncompatibleRebindPreservesConfigurationAndState(t *testing.T) {
	// Arrange.
	registry := sessionConcurrencyRegistry(t, "first")
	session, err := NewSession(registry)
	require.NoError(t, err)
	incompatible, err := NewRegistry()
	require.NoError(t, err)
	if mutationErr := SetSessionState(session, "value", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	before := session.Binding()
	// Act.
	err = session.Rebind(incompatible)
	// Assert.
	require.Error(t, err)
	require.Equal(t, before, session.Binding())
	value, ok := GetSessionState[int](session, "value")
	require.True(t, ok)
	require.Equal(t, 7, value)
	outcome, err := session.RunCall(t.Context(), ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}})
	require.NoError(t, err)
	decoded, err := DecodeOutcomeAs[string](outcome)
	require.NoError(t, err)
	require.Equal(t, "first", *decoded)
}

type reentryMarshalValue struct {
	value    int
	callback func()
}

func (v reentryMarshalValue) MarshalJSON() ([]byte, error) {
	v.callback()
	return json.Marshal(v.value)
}

func TestSnapshotMarshalCallbackReentersState(t *testing.T) {
	// Arrange.
	session, err := NewSession(sessionConcurrencyRegistry(t, "first"))
	require.NoError(t, err)
	if mutationErr := SetSessionState(
		session,
		"value",
		reentryMarshalValue{value: 7, callback: func() {
			if mutationErr := SetSessionState(session, "callback", 1); mutationErr != nil {
				t.Error(mutationErr)
			}
		}},
	); mutationErr != nil {
		t.Error(mutationErr)
	}
	finished := make(chan error, 1)
	// Act.
	go func() { _, exportErr := session.ExportCheckpoint(); finished <- exportErr }()
	// Assert.
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("MarshalJSON callback blocked under a state lock")
	}
	value, ok := GetSessionState[int](session, "callback")
	require.True(t, ok)
	require.Equal(t, 1, value)
}
