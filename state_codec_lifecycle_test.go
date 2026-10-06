package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionFreezesEveryCodecRegistrar(t *testing.T) {
	for _, registrar := range []string{"optional", "required", "typed", "prototype"} {
		t.Run(registrar, func(t *testing.T) {
			// Arrange.
			codecs := NewStateCodecRegistry()
			require.NoError(t, RegisterJSONCodec[int](codecs, "value", WithStateSlotRequired()))
			session, err := NewSession(nil, WithStateCodecRegistry(codecs), WithStrictStateCodecs(true))
			require.NoError(t, err)
			if mutationErr := SetSessionState(session, "value", 7); mutationErr != nil {
				t.Error(mutationErr)
			}
			before := session.Binding()
			// Act.
			var registrationErr error
			switch registrar {
			case "optional":
				registrationErr = RegisterJSONCodec[int](codecs, "late")
			case "required":
				registrationErr = RegisterJSONCodec[int](codecs, "late", WithStateSlotRequired())
			case "typed":
				registrationErr = RegisterStateCodec[int](codecs, "late", JSONStateCodec[int]{})
			case "prototype":
				registrationErr = codecs.RegisterFromPrototype("late", 0)
			}
			checkpoint, err := session.ExportCheckpoint()
			require.NoError(t, err)
			restored, err := NewSessionFromCheckpoint(nil, checkpoint,
				WithStateCodecRegistry(codecs), WithStrictStateCodecs(true))
			// Assert.
			require.ErrorIs(t, registrationErr, ErrStateCodecRegistryFrozen)
			require.NoError(t, err)
			require.Equal(t, before, session.Binding())
			require.Equal(t, before, restored.Binding())
			value, ok := GetSessionState[int](restored, "value")
			require.True(t, ok)
			require.Equal(t, 7, value)
			_, exists := codecs.lookup("late")
			require.False(t, exists)
		})
	}
}

func TestExplicitCodecFreeze(t *testing.T) {
	// Arrange.
	codecs := NewStateCodecRegistry()
	require.NoError(t, codecs.RegisterFromPrototype("value", 0))
	// Act.
	codecs.Freeze()
	codecs.Freeze()
	err := RegisterJSONCodec[int](codecs, "late")
	session, constructionErr := NewSession(nil, WithStateCodecRegistry(codecs))
	// Assert.
	require.ErrorIs(t, err, ErrStateCodecRegistryFrozen)
	require.NoError(t, constructionErr)
	require.NotEmpty(t, session.Binding().StateSchemaDigest)
	var absent *StateCodecRegistry
	absent.Freeze()
}

func TestInvalidSessionDoesNotFreezeCodecBuilder(t *testing.T) {
	for _, failure := range []string{"policy", "registry"} {
		t.Run(failure, func(t *testing.T) {
			// Arrange.
			codecs := NewStateCodecRegistry()
			var registry *Registry
			options := []SessionOption{WithStateCodecRegistry(codecs)}
			if failure == "policy" {
				options = append(options, WithRunPolicy(RunPolicy{AllowedTools: []string{""}}))
			} else {
				tool := newMiddlewareMinTool("value",
					func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error { return nil })
				var err error
				registry, err = NewRegistry(tool)
				require.NoError(t, err)
				tool.manifest.Parameters["invalid"] = make(chan struct{})
			}
			// Act.
			session, err := NewSession(registry, options...)
			registrationErr := RegisterJSONCodec[int](codecs, "still mutable")
			// Assert.
			require.Error(t, err)
			require.Nil(t, session)
			require.NoError(t, registrationErr)
		})
	}
}

type codecRegistrationResult struct {
	key string
	err error
}

func TestConcurrentCodecRegistrationAndSessionConstruction(t *testing.T) {
	for range 10 {
		// Arrange.
		codecs := NewStateCodecRegistry()
		start := make(chan struct{})
		outcomes := make(chan codecRegistrationResult, 40)
		var writers sync.WaitGroup
		for worker := range 4 {
			writers.Go(func() {
				<-start
				for index := range 10 {
					key := fmt.Sprintf("slot-%d-%d", worker, index)
					outcomes <- codecRegistrationResult{key: key, err: RegisterJSONCodec[int](codecs, key, WithStateSlotRequired())}
				}
			})
		}
		// Act: each registration linearizes before or after constructor freeze.
		close(start)
		session, err := NewSession(nil, WithStateCodecRegistry(codecs), WithStrictStateCodecs(true))
		require.NoError(t, err)
		writers.Wait()
		close(outcomes)
		admitted := 0
		for outcome := range outcomes {
			if outcome.err == nil {
				admitted++
				if mutationErr := SetSessionState(session, outcome.key, 7); mutationErr != nil {
					t.Error(mutationErr)
				}
			} else {
				require.ErrorIs(t, outcome.err, ErrStateCodecRegistryFrozen)
			}
		}
		checkpoint, err := session.ExportCheckpoint()
		require.NoError(t, err)
		restored, err := NewSessionFromCheckpoint(
			nil,
			checkpoint,
			WithStateCodecRegistry(codecs),
			WithStrictStateCodecs(true),
		)
		// Assert.
		require.NoError(t, err)
		require.Len(t, codecs.slotPolicies(), admitted)
		require.Equal(t, session.Binding(), restored.Binding())
		require.Equal(t, stateSchemaDigest(codecs), checkpoint.Binding.StateSchemaDigest)
	}
}

func TestRequiredSlotMissingCannotExportCheckpoint(t *testing.T) {
	// Arrange.
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[int](codecs, "required", WithStateSlotRequired()))
	session, err := NewSession(nil, WithStateCodecRegistry(codecs))
	require.NoError(t, err)
	// Act.
	checkpoint, err := session.ExportCheckpoint()
	// Assert.
	require.Error(t, err)
	require.Empty(t, checkpoint)
	require.NoError(t, SetSessionState[any](session, "required", nil))
	_, err = session.ExportSnapshot()
	require.Error(t, err)
	if mutationErr := SetSessionState(session, "required", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	checkpoint, err = session.ExportCheckpoint()
	require.NoError(t, err)
	_, err = NewSessionFromCheckpoint(nil, checkpoint, WithStateCodecRegistry(codecs))
	require.NoError(t, err)
}

func TestCodecSlotOptionCanFinalizeBuilderWithoutDeadlock(t *testing.T) {
	// Arrange.
	codecs := NewStateCodecRegistry()
	finished := make(chan error, 1)
	// Act: host option executes outside the builder's registration lock.
	go func() {
		finished <- RegisterJSONCodec[int](codecs, "value", func(*stateSlotOptions) { codecs.Freeze() })
	}()
	// Assert.
	select {
	case err := <-finished:
		require.ErrorIs(t, err, ErrStateCodecRegistryFrozen)
	case <-time.After(2 * time.Second):
		t.Fatal("state slot option deadlocked")
	}
}

func TestRequiredNullableStateCheckpoint(t *testing.T) {
	for _, nullable := range []bool{false, true} {
		t.Run(strconv.FormatBool(nullable), func(t *testing.T) {
			// Arrange.
			codecs := NewStateCodecRegistry()
			require.NoError(
				t,
				RegisterJSONCodec[*int](codecs, "value", WithStateSlotRequired(), WithStateSlotNullable(nullable)),
			)
			session, err := NewSession(nil, WithStateCodecRegistry(codecs))
			require.NoError(t, err)
			require.NoError(t, SetSessionState[*int](session, "value", nil))
			// Act.
			checkpoint, err := session.ExportCheckpoint()
			// Assert.
			if !nullable {
				require.Error(t, err)
				require.Empty(t, checkpoint)
				return
			}
			require.NoError(t, err)
			restored, err := NewSessionFromCheckpoint(nil, checkpoint, WithStateCodecRegistry(codecs))
			require.NoError(t, err)
			value, exists := restored.state["value"]
			require.True(t, exists)
			require.IsType(t, (*int)(nil), value)
			require.Nil(t, value)
		})
	}
}

func TestCheckpointRestoreUsesCurrentHostPolicyAndBudget(t *testing.T) {
	// Arrange.
	tool := newMiddlewareMinTool("value",
		func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error { return nil })
	other := newMiddlewareMinTool("other",
		func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error { return nil })
	registry, err := NewRegistry(tool, other)
	require.NoError(t, err)
	original, err := NewSession(registry, WithMaxCalls(2), WithRunPolicy(RunPolicy{ForcedTool: "value"}))
	require.NoError(t, err)
	if mutationErr := SetSessionState(original, "persisted", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	call := ToolCall{ToolName: "value", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	yield := func(Chunk) error { return nil }
	for range 2 {
		require.NoError(t, original.Execute(context.Background(), call, yield))
	}
	checkpoint, err := original.ExportCheckpoint()
	require.NoError(t, err)
	// Act.
	restored, err := NewSessionFromCheckpoint(registry, checkpoint,
		WithMaxCalls(1), WithRunPolicy(RunPolicy{ForcedTool: "other"}))
	// Assert.
	require.NoError(t, err)
	require.Equal(t, int64(2), original.Track().CallAttempts())
	require.Zero(t, restored.Track().CallAttempts())
	require.Equal(t, int64(1), restored.Track().MaxCalls())
	require.Error(t, restored.Execute(context.Background(), call, yield))
	require.Zero(t, restored.Track().CallAttempts())
	call.ToolName = "other"
	require.NoError(t, restored.Execute(context.Background(), call, yield))
	require.Error(t, restored.Execute(context.Background(), call, yield))
}

func TestSessionStatePreservesHostAliases(t *testing.T) {
	// Arrange.
	session, err := NewSession(nil)
	require.NoError(t, err)
	value := map[string]int{"count": 1}
	if mutationErr := SetSessionState(session, "value", value); mutationErr != nil {
		t.Error(mutationErr)
	}
	// Act: the host serializes this mutation; state slots do not deep-copy values.
	read, ok := GetSessionState[map[string]int](session, "value")
	read["count"] = 2
	// Assert.
	require.True(t, ok)
	require.Equal(t, 2, value["count"])
}

type failingReentryCodec struct{ callback func() }

func (failingReentryCodec) Encode(value int) ([]byte, error) { return json.Marshal(value) }

func (c failingReentryCodec) Decode([]byte) (int, error) {
	c.callback()
	return 0, errors.New("host decode failed")
}

func TestFailedHydrationDoesNotRollbackHostCallbackWrites(t *testing.T) {
	// Arrange.
	var session *Session
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterStateCodec[int](codecs, "value", failingReentryCodec{callback: func() {
		if mutationErr := SetSessionState(session, "host-write", 9); mutationErr != nil {
			t.Error(mutationErr)
		}
	}}))
	var err error
	session, err = NewSession(nil, WithStateCodecRegistry(codecs))
	require.NoError(t, err)
	if mutationErr := SetSessionState(session, "value", 7); mutationErr != nil {
		t.Error(mutationErr)
	}
	checkpoint, err := session.ExportCheckpoint()
	require.NoError(t, err)
	if mutationErr := SetSessionState(session, "value", 8); mutationErr != nil {
		t.Error(mutationErr)
	}
	finished := make(chan error, 1)
	// Act.
	go func() { finished <- session.ImportSnapshot(checkpoint.Snapshot) }()
	// Assert.
	select {
	case err = <-finished:
		require.Error(t, err)
		require.ErrorContains(t, errors.Unwrap(err), "host decode failed")
	case <-time.After(2 * time.Second):
		t.Fatal("decode callback deadlocked")
	}
	value, ok := GetSessionState[int](session, "value")
	require.True(t, ok)
	require.Equal(t, 8, value)
	write, ok := GetSessionState[int](session, "host-write")
	require.True(t, ok)
	require.Equal(t, 9, write)
}
