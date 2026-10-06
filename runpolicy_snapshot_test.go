package toolsy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func policySnapshotRegistry(t *testing.T) *Registry {
	t.Helper()
	handler := func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error { return nil }
	registry, err := NewRegistry(newMiddlewareMinTool("read", handler), newMiddlewareMinTool("write", handler))
	require.NoError(t, err)
	return registry
}

func TestRunPolicyCaptureAndMaterializationOwnSlices(t *testing.T) {
	// Arrange.
	registry := policySnapshotRegistry(t)
	allowed, required := []string{"read"}, []string{"read"}
	option := WithRunPolicy(RunPolicy{AllowedTools: allowed, CatalogRequiredTools: required})
	// Act: mutations after option capture must not change its validation/admission.
	allowed[0] = "write"
	required[0] = "missing"
	var materialized *sessionOptions
	first, err := NewSession(registry, option, func(cfg *sessionOptions) { materialized = cfg })
	require.NoError(t, err)
	second, err := NewSession(registry, option)
	require.NoError(t, err)
	materialized.policy.AllowedTools[0] = "write"
	materialized.policy.CatalogRequiredTools[0] = "missing"
	first.opts.policy.AllowedTools[0] = "write" // stored options and execution policy are distinct snapshots.
	// Assert.
	for _, session := range []*Session{first, second} {
		require.Equal(t, []string{"read"}, session.policy.AllowedTools)
		require.Equal(t, []string{"read"}, session.policy.CatalogRequiredTools)
		call := ToolCall{ToolName: "read", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
		require.NoError(t, session.Execute(t.Context(), call, func(Chunk) error { return nil }))
		call.ToolName = "write"
		require.Error(t, session.Execute(t.Context(), call, func(Chunk) error { return nil }))
	}
	require.Equal(t, []string{"read"}, second.opts.policy.AllowedTools)
	require.Equal(t, []string{"read"}, first.opts.policy.CatalogRequiredTools)
}

func TestCapturedRunPolicyDoesNotRaceWithCallerMutation(t *testing.T) {
	// Arrange.
	registry := policySnapshotRegistry(t)
	allowed, required := []string{"read"}, []string{"read"}
	option := WithRunPolicy(RunPolicy{AllowedTools: allowed, CatalogRequiredTools: required})
	session, err := NewSession(registry, option)
	require.NoError(t, err)
	var workers sync.WaitGroup
	start := make(chan struct{})
	// Act: the caller owns these original slices; all later library reads use copies.
	workers.Go(func() {
		<-start
		for range 1000 {
			allowed[0] = "write"
			required[0] = "missing"
		}
	})
	for range 4 {
		workers.Go(func() {
			<-start
			for range 100 {
				fresh, buildErr := NewSession(registry, option)
				if buildErr != nil {
					t.Error(buildErr)
					return
				}
				call := ToolCall{ToolName: "read", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
				if callErr := fresh.Execute(t.Context(), call, func(Chunk) error { return nil }); callErr != nil {
					t.Error(callErr)
				}
				if callErr := session.Execute(t.Context(), call, func(Chunk) error { return nil }); callErr != nil {
					t.Error(callErr)
				}
			}
		})
	}
	close(start)
	workers.Wait()
	// Assert.
	require.Equal(t, []string{"read"}, session.policy.AllowedTools)
	require.Equal(t, []string{"read"}, session.policy.CatalogRequiredTools)
}

func TestCatalogRequirementsValidateBeforeCodecFreeze(t *testing.T) {
	for _, registryMode := range []string{"nil", "missing", "view"} {
		t.Run(registryMode, func(t *testing.T) {
			// Arrange.
			var registry *Registry
			if registryMode != "nil" {
				registry = policySnapshotRegistry(t)
			}
			if registryMode == "view" {
				view, err := registry.View(RegistryViewSpec{ToolNames: []string{"read"}})
				require.NoError(t, err)
				registry = view.reg
			}
			codecs := NewStateCodecRegistry()
			required := "absent"
			if registryMode == "view" {
				required = "write"
			}
			// Act.
			session, err := NewSession(
				registry,
				WithStateCodecRegistry(codecs),
				WithRunPolicy(RunPolicy{CatalogRequiredTools: []string{required}}),
			)
			// Assert.
			require.Nil(t, session)
			requireToolErrorCode(t, err, CodeToolsContractMissing)
			require.NoError(t, RegisterJSONCodec[int](codecs, "still-mutable"))
		})
	}
}

func TestNegativeMaxCallsDoesNotFreezeCodecs(t *testing.T) {
	// Arrange.
	codecs := NewStateCodecRegistry()
	// Act.
	session, err := NewSession(nil, WithMaxCalls(-1), WithStateCodecRegistry(codecs))
	// Assert.
	require.Nil(t, session)
	requireToolErrorCode(t, err, CodeValidationFailed)
	require.NoError(t, RegisterJSONCodec[int](codecs, "still-mutable"))
}

func TestMaxCallsAtomicAdmissionAndWire(t *testing.T) {
	// Arrange.
	var executed atomic.Int64
	tool := newMiddlewareMinTool(
		"read",
		func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error { executed.Add(1); return nil },
	)
	registry, err := NewRegistry(tool)
	require.NoError(t, err)
	session, err := NewSession(registry, WithMaxCalls(7))
	require.NoError(t, err)
	var callers sync.WaitGroup
	var denied atomic.Int64
	// Act.
	for range 50 {
		callers.Go(func() {
			callErr := session.Execute(
				t.Context(),
				ToolCall{ToolName: "read", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
				func(Chunk) error { return nil },
			)
			if callErr != nil {
				if te, ok := AsToolError(callErr); !ok || te.Code != CodeMaxCallsExceeded {
					t.Error(callErr)
				}
				denied.Add(1)
			}
		})
	}
	callers.Wait()
	raw, err := marshalToolErrorWire(NewMaxCallsExceededError(), "")
	require.NoError(t, err)
	decoded, err := unmarshalToolErrorWire(raw)
	require.NoError(t, err)
	// Assert.
	require.Equal(t, int64(7), executed.Load())
	require.Equal(t, int64(43), denied.Load())
	require.Equal(t, int64(50), session.Track().CallAttempts())
	require.Equal(t, int64(7), session.Track().MaxCalls())
	require.Equal(t, CodeMaxCallsExceeded, decoded.Code)
	require.ErrorIs(t, decoded, ErrMaxCallsExceeded)
	require.False(t, ClientCorrectable(decoded.Code))
}

func TestCallAttemptAdmissionOrdering(t *testing.T) {
	for _, mode := range []string{"nil-registry", "policy", "unknown-tool", "environment", "cancellation", "arguments", "handler"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			var handled int
			tool, err := NewTool("read", "read", func(context.Context, *RunEnv, struct {
				N int `json:"n"`
			}) (int, error) {
				handled++
				return 0, NewInternalError(context.DeadlineExceeded)
			})
			require.NoError(t, err)
			registry, err := NewRegistry(tool)
			require.NoError(t, err)
			options := []SessionOption{WithMaxCalls(1)}
			if mode == "nil-registry" {
				registry = nil
			}
			if mode == "policy" {
				options = append(options, WithRunPolicy(RunPolicy{AllowedTools: []string{"write"}}))
			}
			session, err := NewSession(registry, options...)
			require.NoError(t, err)
			call := ToolCall{ToolName: "read", Input: ToolInput{ArgsJSON: []byte(`{"n":1}`)}}
			ctx := t.Context()
			switch mode {
			case "unknown-tool":
				call.ToolName = "absent"
			case "environment":
				other, buildErr := NewSession(registry)
				require.NoError(t, buildErr)
				call.Env = NewRunEnv(other)
			case "cancellation":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "arguments":
				call.Input.ArgsJSON = []byte(`{"n":"bad"}`)
			}
			// Act.
			err = session.Execute(ctx, call, func(Chunk) error { return nil })
			// Assert.
			require.Error(t, err)
			expected := int64(1)
			if mode == "nil-registry" || mode == "policy" {
				expected = 0
			}
			require.Equal(t, expected, session.Track().CallAttempts())
			if mode == "handler" {
				require.Equal(t, 1, handled)
			} else {
				require.Zero(t, handled)
			}
		})
	}
}
