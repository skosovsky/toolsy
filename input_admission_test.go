package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type admissionArgs struct {
	Text   string              `json:"text"`
	Number int64               `json:"number"`
	Nested map[string][]string `json:"nested"`
}

func admissionTyped(t *testing.T, binder ArgsBinder[admissionArgs], validator ArgValidator[admissionArgs]) Tool {
	t.Helper()
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, admissionArgs, string, string]{
		Name:         "typed",
		ArgsBinder:   binder,
		ArgValidator: validator,
		Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[admissionArgs]) (ToolResult[string, string], error) {
			return NewToolResult[string, string]("ok"), nil
		},
	})
	require.NoError(t, err)
	return tool
}

func admissionDynamic(t *testing.T, validator func(context.Context, map[string]any) error) Tool {
	t.Helper()
	tool, err := NewDynamicToolFromSpec(DynamicToolSpec{
		Name: "dynamic", Schema: MapSchemaProvider{"type": "object", "properties": map[string]any{
			"text":   map[string]any{"type": "string"},
			"number": map[string]any{"type": "integer"},
			"nested": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		}, "required": []string{"text", "number", "nested"}},
		ValidateArgs: validator,
		Handler: func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`"ok"`), MimeType: MimeTypeJSON})
		},
	})
	require.NoError(t, err)
	return tool
}

func TestInputAdmissionCanonicalParityAndIsolation(t *testing.T) {
	// Arrange: sanitizers change Value while preserving original Raw.
	binder := func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
		var args admissionArgs
		if err := json.Unmarshal(req.Input.ArgsJSON, &args); err != nil {
			return ValidatedArgs[admissionArgs]{}, err
		}
		args.Text = strings.TrimSpace(args.Text)
		req.Manifest.Parameters["type"] = "string"
		req.CallContext.Values["nested"].(map[string]any)["x"] = "changed"
		return ValidatedArgs[admissionArgs]{Value: args, Raw: req.Input.ArgsJSON}, nil
	}
	tools := map[string]Tool{
		"typed binder":    admissionTyped(t, binder, nil),
		"typed extractor": admissionTyped(t, nil, nil),
		"dynamic sanitizer": admissionDynamic(t, func(_ context.Context, args map[string]any) error {
			args["text"] = strings.TrimSpace(args["text"].(string))
			return nil
		}),
	}
	for name, tool := range tools {
		t.Run(name, func(t *testing.T) {
			// Arrange.
			in := ToolInput{
				CallID:      "same",
				ArgsJSON:    []byte(`{"text":" hi ","number":9007199254740993,"nested":{"items":["a","b"]}}`),
				Attachments: []Attachment{{Data: []byte("attachment")}},
			}
			original := in.Clone()
			call := CallContext{Values: map[string]any{"nested": map[string]any{"x": "original"}}}
			var observed ToolInput
			profile := executionProfileFunc(
				func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
					observed = call.Input.Clone()
					return invoke(yield)
				},
			)
			// Act.
			admitted, err := AdmitInput(context.Background(), tool, call, in)
			require.NoError(t, err)
			require.NoError(
				t,
				tool.Execute(
					context.Background(),
					NewRunEnv(nil, WithRunCallContext(call), WithRunExecutionProfile(profile)),
					in,
					func(Chunk) error { return nil },
				),
			)
			admitted.ArgsJSON[0] = 'x'
			admitted.Attachments[0].Data[0] = 'x'
			again, againErr := AdmitInput(context.Background(), tool, call, in)
			// Assert.
			require.NoError(t, againErr)
			assert.Equal(t, observed.ArgsJSON, again.ArgsJSON)
			assert.Contains(t, string(again.ArgsJSON), "9007199254740993")
			assert.Equal(t, original, in)
			assert.Equal(t, "object", tool.Manifest().Parameters["type"])
			assert.Equal(t, "original", call.Values["nested"].(map[string]any)["x"])
		})
	}
}

func TestInputAdmissionErrorParity(t *testing.T) {
	for _, kind := range []string{"typed schema", "binder", "validator", "dynamic schema", "dynamic validator", "context", "encoding"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange.
			tool := admissionTyped(t, nil, nil)
			in := ToolInput{ArgsJSON: []byte(`{}`)}
			call := CallContext{}
			switch kind {
			case "typed schema":
			case "binder":
				tool = admissionTyped(t, func(context.Context, ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
					return ValidatedArgs[admissionArgs]{}, errors.New("bad binder")
				}, nil)
			case "validator":
				tool = admissionTyped(t, func(context.Context, ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
					return ValidatedArgs[admissionArgs]{}, nil
				}, func(admissionArgs) error { return errors.New("bad value") })
			case "dynamic schema":
				tool = admissionDynamic(t, nil)
			case "dynamic validator":
				tool = admissionDynamic(
					t,
					func(context.Context, map[string]any) error { return errors.New("bad value") },
				)
				in.ArgsJSON = []byte(`{"text":"x","number":1,"nested":{}}`)
			case "context":
				call.Subject = "wrong type"
			case "encoding":
				tool = admissionDynamic(
					t,
					func(_ context.Context, args map[string]any) error { args["text"] = func() {}; return nil },
				)
				in.ArgsJSON = []byte(`{"text":"x","number":1,"nested":{}}`)
			}
			var profiles int
			profile := executionProfileFunc(
				func(context.Context, PreparedCall, InvocationHandler, func(Chunk) error) error {
					profiles++
					return nil
				},
			)
			// Act.
			_, admitErr := AdmitInput(context.Background(), tool, call, in)
			executeErr := tool.Execute(
				context.Background(),
				NewRunEnv(nil, WithRunCallContext(call), WithRunExecutionProfile(profile)),
				in,
				func(Chunk) error { return nil },
			)
			// Assert: compare contract type/code, not incidental diagnostic strings.
			require.Error(t, admitErr)
			require.Error(t, executeErr)
			assert.Equal(t, reflect.TypeOf(executeErr), reflect.TypeOf(admitErr))
			admittedError, ok := AsToolError(admitErr)
			require.True(t, ok)
			executionError, ok := AsToolError(executeErr)
			require.True(t, ok)
			assert.Equal(t, executionError.Code, admittedError.Code)
			assert.Equal(t, executionError.Retryable, admittedError.Retryable)
			assert.Equal(t, executionError.FixableArgs, admittedError.FixableArgs)
			assert.Equal(t, reflect.TypeOf(executionError.Err), reflect.TypeOf(admittedError.Err))
			assert.Zero(t, profiles)
		})
	}
}

type admissionOpaque struct{ next Tool }

func (t *admissionOpaque) Manifest() ToolManifest { return t.next.Manifest() }
func (t *admissionOpaque) Execute(ctx context.Context, env *RunEnv, in ToolInput, yield func(Chunk) error) error {
	in.ArgsJSON = []byte(`{}`)
	return t.next.Execute(ctx, env, in, yield)
}
func (t *admissionOpaque) UnwrapNext() Tool { return t.next }

type admissionModifier struct {
	admissionOpaque

	replacement []byte
}

func (t *admissionModifier) Execute(ctx context.Context, env *RunEnv, in ToolInput, yield func(Chunk) error) error {
	in.ArgsJSON = t.replacement
	return t.next.Execute(ctx, env, in, yield)
}

func (t *admissionModifier) AdmitInput(ctx context.Context, call CallContext, in ToolInput) (ToolInput, error) {
	in.ArgsJSON = t.replacement
	return AdmitInput(ctx, t.next, call, in)
}

func TestInputAdmissionWrappers(t *testing.T) {
	// Arrange.
	base := admissionTyped(t, nil, nil)
	valid := ToolInput{ArgsJSON: []byte(`{"text":"x","number":1,"nested":{}}`)}
	logger := slog.New(slog.DiscardHandler)
	wrapped := WithLogging(
		logger,
	)(
		WithRecovery()(
			WithBudget()(WithTruncation(100)(WithErrorFormatter()(OverrideTool(base, WithNewName("alias"))))),
		),
	)
	cycle := &recoveryTool{}
	cycle.next = cycle
	deep := base
	for range maxInputAdmissionLayers {
		deep = WithRecovery()(deep)
	}
	var nilTool *tool
	lowLevel, err := NewTool("low", "", func(context.Context, *RunEnv, struct{}) (string, error) { return "", nil })
	require.NoError(t, err)
	// Act / Assert.
	_, err = AdmitInput(context.Background(), wrapped, CallContext{}, valid)
	require.NoError(t, err)
	_, err = AdmitInput(context.Background(), AsAsyncTool(base), CallContext{}, valid)
	require.NoError(t, err)
	for _, unsupported := range []Tool{nil, nilTool, cycle, deep, lowLevel, &admissionOpaque{next: base}} {
		_, err = AdmitInput(context.Background(), unsupported, CallContext{}, valid)
		require.ErrorIs(t, err, ErrInputAdmissionUnsupported)
	}
	_, err = AdmitInput(
		context.Background(),
		&admissionModifier{
			admissionOpaque: admissionOpaque{next: base},
			replacement:     []byte(`{}`),
		},
		CallContext{},
		valid,
	)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInputAdmissionUnsupported)
	supported := &admissionModifier{
		admissionOpaque: admissionOpaque{next: base},
		replacement:     valid.ArgsJSON,
	}
	snapshot, err := AdmitInput(context.Background(), supported, CallContext{}, ToolInput{ArgsJSON: []byte(`{}`)})
	require.NoError(t, err)
	assert.JSONEq(t, string(valid.ArgsJSON), string(snapshot.ArgsJSON))
	var observed ToolInput
	profile := executionProfileFunc(
		func(_ context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			observed = call.Input.Clone()
			return invoke(yield)
		},
	)
	require.NoError(
		t,
		supported.Execute(
			context.Background(),
			NewRunEnv(nil, WithRunExecutionProfile(profile)),
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { return nil },
		),
	)
	assert.Equal(t, observed.ArgsJSON, snapshot.ArgsJSON)
}

func TestInputAdmissionCancellationAndPanic(t *testing.T) {
	// Arrange.
	var binds int
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	binder := func(context.Context, ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
		binds++
		return ValidatedArgs[admissionArgs]{}, nil
	}
	tool := admissionTyped(t, binder, nil)
	// Act.
	_, err := AdmitInput(canceled, tool, CallContext{}, ToolInput{})
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, binds)
	t.Run("during binder", func(t *testing.T) {
		// Arrange.
		ctx, cancelDuring := context.WithCancel(context.Background())
		defer cancelDuring()
		tool := admissionTyped(t, func(context.Context, ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
			cancelDuring()
			return ValidatedArgs[admissionArgs]{}, nil
		}, nil)
		// Act.
		_, err := AdmitInput(ctx, tool, CallContext{}, ToolInput{})
		// Assert.
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("panic propagates", func(t *testing.T) {
		// Arrange.
		tool := WithRecovery()(
			admissionTyped(
				t,
				func(context.Context, ArgsBindRequest) (ValidatedArgs[admissionArgs], error) { panic("binder panic") },
				nil,
			),
		)
		// Act / Assert.
		require.PanicsWithValue(
			t,
			"binder panic",
			func() { _, _ = AdmitInput(context.Background(), tool, CallContext{}, ToolInput{}) },
		)
	})
}

func TestInputAdmissionDispatchRechecks(t *testing.T) {
	// Arrange.
	var handlers, policies int
	denied := false
	tool, err := NewTypedTool(TypedToolSpec[string, string, admissionArgs, string, string]{
		Name: "guarded",
		Policy: func(_ context.Context, req TypedPolicyRequest[string, string, admissionArgs]) Decision {
			policies++
			if denied || req.Context.Scope != "allowed" {
				return DenyDecision("revoked")
			}
			return AllowDecision()
		},
		Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[admissionArgs]) (ToolResult[string, string], error) {
			handlers++
			return NewToolResult[string, string]("ok"), nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)
	in := ToolInput{CallID: "same", ArgsJSON: []byte(`{"text":"x","number":1,"nested":{}}`)}
	call := NewCallContext("alice", "allowed")
	// Act.
	snapshot, err := AdmitInput(context.Background(), tool, call, in)
	require.NoError(t, err)
	assert.Zero(t, policies)
	denied = true
	deniedErr := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "guarded", CallContext: call, Input: snapshot},
		func(Chunk) error { return nil },
	)
	denied = false
	scopeErr := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "guarded", CallContext: NewCallContext("alice", "revoked"), Input: snapshot},
		func(Chunk) error { return nil },
	)
	snapshot.ArgsJSON = []byte(`{}`)
	argsErr := reg.Execute(
		context.Background(),
		ToolCall{ToolName: "guarded", CallContext: call, Input: snapshot},
		func(Chunk) error { return nil },
	)
	// Assert.
	require.ErrorIs(t, deniedErr, ErrPolicyDenied)
	require.ErrorIs(t, scopeErr, ErrPolicyDenied)
	require.Error(t, argsErr)
	assert.Zero(t, handlers)
	assert.Equal(t, 2, policies)
}

func TestInputAdmissionConcurrentSnapshots(t *testing.T) {
	// Arrange.
	tool := admissionTyped(t, nil, nil)
	in := ToolInput{ArgsJSON: []byte(`{"text":"x","number":9007199254740993,"nested":{"items":["a"]}}`)}
	var wg sync.WaitGroup
	// Act.
	for range 16 {
		wg.Go(func() {
			out, err := AdmitInput(context.Background(), tool, CallContext{}, in)
			if assert.NoError(t, err) {
				out.ArgsJSON[0] = 'x'
			}
		})
	}
	wg.Wait()
	// Assert.
	assert.Equal(t, byte('{'), in.ArgsJSON[0])
}

type admissionClaimStore struct {
	OperationStore

	claims int
}

func (s *admissionClaimStore) Claim(ctx context.Context, claim OperationClaim) (ClaimResult, error) {
	s.claims++
	return s.OperationStore.Claim(ctx, claim)
}

func TestInputAdmissionWholeBatch(t *testing.T) {
	// Arrange: the host has real operation journaling and budget middleware installed.
	var handlers, binds, profiles, controls int
	store := &admissionClaimStore{OperationStore: NewMemoryOperationStore()}
	operation, err := NewOperationProfile(OperationProfileConfig{
		Store: store, Prepare: func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{
				Namespace:         "batch",
				Scope:             "tenant",
				Subject:           "host",
				OperationID:       call.Input.CallID,
				AttemptID:         "first",
				PolicyFingerprint: "policy",
				CanonicalDigest:   "digest",
				CanonicalRules:    "json",
				DisplayJSON:       []byte(`{}`),
			}, nil
		}, Codec: JSONResultCodec[string, string]{}, Issuer: "host", Clock: time.Now, Lease: time.Minute,
	})
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(ctx context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			profiles++
			return operation.ExecutePrepared(ctx, call, invoke, yield)
		},
	)
	typed, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, admissionArgs, string, string]{
		Name: "first", ArgsBinder: func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[admissionArgs], error) {
			binds++
			var args admissionArgs
			decodeErr := json.Unmarshal(req.Input.ArgsJSON, &args)
			return ValidatedArgs[admissionArgs]{Value: args}, decodeErr
		}, Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[admissionArgs]) (ToolResult[string, string], error) {
			handlers++
			res := NewToolResult[string, string]("ok")
			res.Controls = []ControlSignal{&HostEventSignal{Name: "receipt"}}
			return res, nil
		},
	})
	require.NoError(t, err)
	dynamic, err := NewDynamicToolFromSpec(DynamicToolSpec{
		Name: "second",
		Schema: MapSchemaProvider{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []string{"text"},
		},
		Handler: func(_ context.Context, _ *RunEnv, _ map[string]any, yield func(Chunk) error) error {
			handlers++
			return yield(Chunk{Event: EventResult, TypedResult: "ok", Data: []byte(`"ok"`), MimeType: MimeTypeJSON})
		},
	})
	require.NoError(t, err)
	tools := []Tool{WithBudget()(typed), WithBudget()(dynamic)}
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tools...).Build()
	require.NoError(t, err)
	budget := &testBudgetTracker{}
	calls := []ToolCall{
		{
			ToolName: "first",
			Input:    ToolInput{CallID: "first", ArgsJSON: []byte(`{"text":"ok","number":1,"nested":{}}`)},
			Env:      budgetEnv(budget),
		},
		{ToolName: "second", Input: ToolInput{CallID: "second", ArgsJSON: []byte(`{}`)}, Env: budgetEnv(budget)},
	}
	executeBatch := func() error {
		for i, tool := range tools {
			if _, err := AdmitInput(context.Background(), tool, calls[i].CallContext, calls[i].Input); err != nil {
				return err
			}
		}
		for _, call := range calls {
			if err := reg.Execute(context.Background(), call, func(c Chunk) error {
				controls += len(c.Controls)
				if c.Event == EventControl {
					controls++
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}
	// Act: reject the second slot before entering the execution phase.
	rejected := executeBatch()
	// Assert.
	require.Error(t, rejected)
	assert.Positive(t, binds)
	assert.Zero(t, handlers)
	assert.Zero(t, profiles)
	assert.Zero(t, store.claims)
	assert.Zero(t, controls)
	assert.Zero(t, budget.calls.Load())
	// Arrange: both slots now pass input admission.
	calls[1].Input.ArgsJSON = []byte(`{"text":"ok"}`)
	// Act.
	accepted := executeBatch()
	// Assert: only phase two claims, spends budget and delivers controls.
	require.NoError(t, accepted)
	assert.Equal(t, 2, handlers)
	assert.Equal(t, 2, profiles)
	assert.Equal(t, 2, store.claims)
	assert.Positive(t, controls)
	assert.EqualValues(t, 2, budget.calls.Load())
	assert.Equal(t, 3, binds)
}
