package toolsy

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestPerformanceFreshStickyRejection(t *testing.T) {
	for _, withCheck := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "prepared"}[withCheck], func(t *testing.T) {
			// Arrange: a producer ignores rejection and tries to follow it with valid data.
			dispatched, delivered := 0, 0
			tool, err := NewProxyTool(
				"sticky",
				"sticky",
				[]byte(`{"type":"object"}`),
				func(_ context.Context, _ *RunEnv, _ []byte, yield func(Chunk) error) error {
					dispatched++
					_ = yield(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: []byte(`"wrong"`)})
					_ = yield(Chunk{Event: EventResult, MimeType: MimeTypeJSON, Data: []byte(`1`)})
					return nil
				},
				WithOutputSchema(map[string]any{"type": "integer"}),
			)
			if err != nil {
				t.Fatal(err)
			}
			env := NewRunEnv(nil)
			if withCheck {
				env.preparedChecks = []func(context.Context, PreparedCall, any) error{
					func(context.Context, PreparedCall, any) error { return nil },
				}
			}
			// Act.
			err = tool.Execute(
				context.Background(),
				env,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(Chunk) error { delivered++; return nil },
			)
			// Assert.
			var contract *ResultContractError
			if !errors.As(err, &contract) || contract.Kind != "schema_mismatch" || dispatched != 1 || delivered != 0 {
				t.Fatalf("rejection lost: %v, dispatch=%d delivery=%d", err, dispatched, delivered)
			}
		})
	}
}

func TestPerformanceFreshPreparedCancellation(t *testing.T) {
	// Arrange: authorization check cancels before handler dispatch.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	calls := 0
	tool, err := NewTool(
		"cancel",
		"cancel",
		func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "value", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	env := NewRunEnv(nil)
	env.preparedChecks = []func(context.Context, PreparedCall, any) error{
		func(context.Context, PreparedCall, any) error { cancel(); return ctx.Err() },
	}
	// Act.
	err = tool.Execute(ctx, env, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
	// Assert.
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("cancel bypassed: %v, dispatch=%d", err, calls)
	}
}

type performanceCorruptTool struct{ toolBase }

func (t *performanceCorruptTool) Execute(
	ctx context.Context,
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	return t.next.Execute(ctx, env, input, func(c Chunk) error {
		c.Data = []byte(`{"invalid`)
		c.Envelope = nil
		return yield(c)
	})
}

func TestPerformanceOuterWrapperRevalidation(t *testing.T) {
	// Arrange: a wrapper corrupts the wire after builtin result validation.
	base, err := NewTool("outer", "outer", func(context.Context, *RunEnv, struct{}) (int, error) { return 1, nil })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry(&performanceCorruptTool{toolBase{next: base}})
	if err != nil {
		t.Fatal(err)
	}
	delivered := 0
	// Act.
	err = registry.Execute(
		context.Background(),
		ToolCall{ToolName: "outer", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { delivered++; return nil },
	)
	// Assert.
	var contract *ResultContractError
	if !errors.As(err, &contract) || contract.Kind != resultInvalidJSON || delivered != 0 {
		t.Fatalf("wrapper bypassed outer validation: %v", err)
	}
}

func TestPerformanceBuiltinPolicySnapshot(t *testing.T) {
	// Arrange: policy tries to poison constructor-owned schema and input.
	base, err := NewProxyTool(
		"policy",
		"policy",
		[]byte(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`),
		func(_ context.Context, _ *RunEnv, input []byte, _ func(Chunk) error) error {
			if string(input) != `{"n":1}` {
				return errors.New("policy changed handler input")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
		req.Manifest.Parameters["properties"].(map[string]any)["n"].(map[string]any)["type"] = "string"
		req.Input.ArgsJSON[5] = '9'
		return AllowDecision()
	})
	registry, err := NewRegistryBuilder(WithPolicy("snapshot", policy)).Add(base).Build()
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	for range 2 {
		err = registry.Execute(
			context.Background(),
			ToolCall{ToolName: "policy", Input: ToolInput{ArgsJSON: []byte(`{"n":1}`)}},
			func(Chunk) error { return nil },
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Assert.
	if base.Manifest().Parameters["properties"].(map[string]any)["n"].(map[string]any)["type"] != "integer" {
		t.Fatal("policy mutated frozen builtin")
	}
}

type performanceNameTool struct {
	toolBase

	calls *int
}

func (t *performanceNameTool) Manifest() ToolManifest {
	*t.calls++
	m := t.next.Manifest()
	m.Name = "custom"
	return m
}

func (t *performanceNameTool) Execute(
	ctx context.Context,
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	return t.next.Execute(ctx, env, input, yield)
}

func TestPerformanceLoggingEffectiveName(t *testing.T) {
	// Arrange: only the exact builtin can avoid a public Manifest callback.
	base, err := NewProxyTool(
		"builtin",
		"builtin",
		[]byte(`{"type":"object"}`),
		func(context.Context, *RunEnv, []byte, func(Chunk) error) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	custom := &performanceNameTool{calls: &calls}
	custom.next = base
	logging := WithLogging(slog.New(slog.DiscardHandler))(custom)
	// Act.
	err = logging.Execute(
		context.Background(),
		nil,
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	builtinName := loggingToolName(base)
	overrideName := loggingToolName(OverrideTool(base, WithNewName("alias")))
	// Assert.
	if err != nil || calls != 1 || builtinName != "builtin" || overrideName != "alias" {
		t.Fatalf("effective name/callback bypassed: %v calls=%d names=%q/%q", err, calls, builtinName, overrideName)
	}
}

type performanceMutableManifestTool struct {
	toolBase

	value ToolManifest
}

func (t *performanceMutableManifestTool) Manifest() ToolManifest { return t.value }

func (t *performanceMutableManifestTool) Execute(
	ctx context.Context,
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	return t.next.Execute(ctx, env, input, yield)
}
func TestPerformancePolicyFacadeManifestOwnership(t *testing.T) {
	// Arrange: an exact internal policy facade can wrap a custom live manifest.
	base, err := NewProxyTool(
		"base",
		"base",
		[]byte(`{"type":"object"}`),
		func(context.Context, *RunEnv, []byte, func(Chunk) error) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	custom := &performanceMutableManifestTool{
		value: ToolManifest{
			Name: "facade",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"x": map[string]any{"type": "string"}},
			},
		},
	}
	custom.next = base
	facade, err := NewPolicyTool(
		ToolPolicySpec[NoSubject, NoScope, struct{}]{
			Tool: custom,
			ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
				return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Act: mutate the private execution snapshot; it must not be an immutable borrow.
	captured := executionManifest(facade)
	captured.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] = "integer"
	// Assert: custom facade data is protected, while true constructor ownership is explicit.
	if custom.value.Parameters["properties"].(map[string]any)["x"].(map[string]any)["type"] != "string" {
		t.Fatal("borrow crossed custom facade ownership")
	}
	if facade.(*tool).manifestOwned || !base.(*tool).manifestOwned {
		t.Fatal("incorrect immutable owner classification")
	}
}
