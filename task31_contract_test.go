package toolsy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type validatorFunc func(context.Context, string, string) error

func (f validatorFunc) Validate(ctx context.Context, toolName string, argsJSON string) error {
	return f(ctx, toolName, argsJSON)
}

func TestTask31TypedToolPipeline_ContextPolicyOutcomeEffects(t *testing.T) {
	t.Parallel()
	type subject struct {
		ID string
	}
	type scope struct {
		Tenant string
	}
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		V int `json:"v"`
	}
	type effect struct {
		Kind string
	}

	// Arrange.
	var rawSeen, policySeen, handlerSeen bool
	tool, err := NewTypedTool(TypedToolSpec[subject, scope, args, result, effect]{
		Name:        "typed_task31",
		Description: "Typed task31 contract",
		ArgsBinder: func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[args], error) {
			rawSeen = req.Manifest.Name == "typed_task31" && strings.Contains(string(req.Input.ArgsJSON), `"n":2`)
			return ValidatedArgs[args]{
				Value: args{N: 2},
				Raw:   req.Input.ArgsJSON,
			}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[subject, scope, args]) Decision {
			policySeen = req.Context.Subject.ID == "u1" &&
				req.Context.Scope.Tenant == "t1" &&
				req.Args.N == 2
			return AllowDecision()
		},
		ArgValidator: func(a args) error {
			if a.N <= 0 {
				return NewValidationError("n must be positive")
			}
			return nil
		},
		ResultValidator: func(r result) error {
			if r.V <= 0 {
				return NewValidationError("result must be positive")
			}
			return nil
		},
		EffectValidator: func(effects []effect) error {
			if len(effects) != 1 || effects[0].Kind != "indexed" {
				return NewValidationError("unexpected effects")
			}
			return nil
		},
		Postcondition: func(out ToolResult[result, effect]) error {
			if out.Value.V == 0 || len(out.Effects) == 0 || len(out.Controls) == 0 {
				return NewValidationError("incomplete result envelope")
			}
			return nil
		},
		Handler: func(
			_ context.Context,
			call TypedCallContext[subject, scope],
			_ *RunEnv,
			a ValidatedArgs[args],
		) (ToolResult[result, effect], error) {
			handlerSeen = call.Subject.ID == "u1" && call.Scope.Tenant == "t1"
			out := NewToolResult[result, effect](result{V: a.Value.N * 10})
			out.Effects = []effect{{Kind: "indexed"}}
			out.Controls = []ControlSignal{&UIActionSignal{Action: "refresh"}}
			return out, nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	// Act.
	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "typed_task31",
		Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{"n":2}`)},
		Env:      NewRunEnv(sess),
		CallContext: NewCallContext(
			subject{ID: "u1"},
			scope{Tenant: "t1"},
		),
	})

	// Assert.
	require.NoError(t, err)
	require.Nil(t, outcome.ExecutionError)
	assert.True(t, rawSeen)
	assert.True(t, policySeen)
	assert.True(t, handlerSeen)
	assert.Equal(t, OutcomeSuccess, outcome.Status)
	decoded, err := DecodeOutcomeAs[result](outcome)
	require.NoError(t, err)
	assert.Equal(t, result{V: 20}, *decoded)
	effects, err := DecodeOutcomeEffectsAs[effect](outcome)
	require.NoError(t, err)
	assert.Equal(t, []effect{{Kind: "indexed"}}, effects)
	require.Len(t, outcome.Controls, 1)
	assert.IsType(t, &UIActionSignal{}, outcome.Controls[0])
}

func TestTask31RegistryPolicy_DeniesBeforeValidatorAndHandler(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		V int `json:"v"`
	}

	// Arrange.
	var validatorRan, handlerRan bool
	tool, err := NewTool("deny_first", "Denied", func(_ context.Context, _ *RunEnv, _ args) (result, error) {
		handlerRan = true
		return result{V: 1}, nil
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithPolicy("task31-deny-policy", PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
			require.Equal(t, "deny_first", req.Manifest.Name)
			return DenyDecision("blocked by policy")
		})),
		WithValidator(validatorFunc(func(_ context.Context, _ string, _ string) error {
			validatorRan = true
			return nil
		})),
	).Add(tool).Build()
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "deny_first",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodePolicyDenied, te.Code)
	assert.False(t, validatorRan)
	assert.False(t, handlerRan)
}

func TestTask31RequirementsPolicy_EnforcesManifestRequirementsWithTypedContext(t *testing.T) {
	t.Parallel()
	type subject struct {
		ID          string
		Permissions map[Permission]bool
	}
	type scope struct {
		Workspace string
	}
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		OK bool `json:"ok"`
	}

	// Arrange.
	var policySawRequirements, validatorRan, handlerRan bool
	tool, err := NewTool(
		"write_state",
		"Write state",
		func(_ context.Context, _ *RunEnv, _ args) (result, error) {
			handlerRan = true
			return result{OK: true}, nil
		},
		WithRequirements(ToolRequirements{
			MemoryAccess: MemoryAccessReadWrite,
			NeedsSession: true,
			Permissions:  []Permission{"edit"},
		}),
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithRequirementsPolicy("task31-requirements-policy", func(
			_ context.Context,
			req RequirementsPolicyRequest[subject, scope],
		) Decision {
			policySawRequirements = req.Manifest.Name == "write_state" &&
				req.Requirements.MemoryAccess == MemoryAccessReadWrite &&
				req.Requirements.NeedsSession &&
				assert.ObjectsAreEqual([]Permission{"edit"}, req.Requirements.Permissions) &&
				req.Context.Subject.ID == "u1" &&
				req.Context.Scope.Workspace == "w1"
			if !req.Context.Subject.Permissions["edit"] {
				return DenyDecision("missing permission", "permissions")
			}
			return AllowDecision()
		}),
		WithValidator(validatorFunc(func(_ context.Context, _ string, _ string) error {
			validatorRan = true
			return nil
		})),
	).Add(tool).Build()
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "write_state",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
		CallContext: NewCallContext(
			subject{ID: "u1", Permissions: map[Permission]bool{}},
			scope{Workspace: "w1"},
		),
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodePolicyDenied, te.Code)
	assert.True(t, policySawRequirements)
	assert.False(t, validatorRan)
	assert.False(t, handlerRan)
}

func TestTask31Requirements_DeclaredRequirementsRequireRequirementsPolicy(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		OK bool `json:"ok"`
	}

	cases := []struct {
		name string
		req  ToolRequirements
	}{
		{name: "permissions", req: ToolRequirements{Permissions: []Permission{"admin"}}},
		{name: "memory", req: ToolRequirements{MemoryAccess: MemoryAccessRead}},
		{name: "session", req: ToolRequirements{NeedsSession: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange.
			var validatorRan, handlerRan bool
			tool, err := NewTool(
				"admin_write_"+tc.name,
				"Admin write",
				func(_ context.Context, _ *RunEnv, _ args) (result, error) {
					handlerRan = true
					return result{OK: true}, nil
				},
				WithRequirements(tc.req),
			)
			require.NoError(t, err)
			reg, err := NewRegistryBuilder(
				WithValidator(validatorFunc(func(_ context.Context, _ string, _ string) error {
					validatorRan = true
					return nil
				})),
			).Add(tool).Build()
			require.NoError(t, err)

			// Act.
			err = reg.Execute(context.Background(), ToolCall{
				ToolName: tool.Manifest().Name,
				Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
			}, func(Chunk) error { return nil })

			// Assert.
			require.Error(t, err)
			te, ok := AsToolError(err)
			require.True(t, ok)
			assert.Equal(t, CodePolicyDenied, te.Code)
			assert.False(t, validatorRan)
			assert.False(t, handlerRan)
		})
	}
}

func TestTask31RequirementsPolicy_AllowsToolsWithoutRequirementsWithoutTypedContext(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		OK bool `json:"ok"`
	}

	// Arrange.
	var handlerRan bool
	tool, err := NewTool("plain", "Plain", func(_ context.Context, _ *RunEnv, _ args) (result, error) {
		handlerRan = true
		return result{OK: true}, nil
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithRequirementsPolicy("task31-plain-requirements-policy", func(
			_ context.Context,
			_ RequirementsPolicyRequest[struct{ ID string }, struct{ Tenant string }],
		) Decision {
			t.Fatal("requirements policy must not require typed context for tools without requirements")
			return DenyDecision("unexpected")
		}),
	).Add(tool).Build()
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "plain",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.NoError(t, err)
	assert.True(t, handlerRan)
}

func TestTask31PolicyAndAuthorizer_ReceiveDefensiveInputCopies(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		N int `json:"n"`
	}

	// Arrange.
	var handlerSaw int
	mutateInput := func(in ToolInput) {
		if len(in.ArgsJSON) > 5 {
			in.ArgsJSON[5] = '9'
		}
		if len(in.Attachments) > 0 && len(in.Attachments[0].Data) > 0 {
			in.Attachments[0].Data[0] = 'x'
		}
	}
	tool, err := NewTool(
		"copy_input",
		"Copy input",
		func(_ context.Context, _ *RunEnv, a args) (result, error) {
			handlerSaw = a.N
			return result(a), nil
		},
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithAuthorizer(AuthorizerFunc(func(_ context.Context, req AuthorizationRequest) error {
			mutateInput(req.Input)
			return nil
		})),
		WithPolicy("task31-copy-policy", PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
			mutateInput(req.Input)
			return AllowDecision()
		})),
	).Add(tool).Build()
	require.NoError(t, err)
	input := ToolInput{
		ArgsJSON:    []byte(`{"n":1}`),
		Attachments: []Attachment{{MimeType: MimeTypeText, Data: []byte("safe")}},
	}

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "copy_input",
		Input:    input,
	}, func(Chunk) error { return nil })

	// Assert.
	require.NoError(t, err)
	assert.Equal(t, 1, handlerSaw)
	assert.Equal(t, []byte(`{"n":1}`), input.ArgsJSON)
	assert.Equal(t, []byte("safe"), input.Attachments[0].Data)
}

func TestTask31TypedPipeline_ReceivesDefensiveCopiesAndSystemOwnedViewID(t *testing.T) {
	t.Parallel()
	type args struct {
		Items []int `json:"items"`
	}
	type result struct {
		First int `json:"first"`
	}

	// Arrange.
	var policyViewID, handlerViewID string
	var handlerFirst int
	mutateFirstDigit := func(raw []byte, replacement byte) {
		for i := range raw {
			if raw[i] >= '0' && raw[i] <= '9' {
				raw[i] = replacement
				return
			}
		}
	}
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, result, struct{}]{
		Name:        "typed_copy",
		Description: "Typed copy",
		ArgsBinder: func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[args], error) {
			raw := append([]byte(nil), req.Input.ArgsJSON...)
			mutateFirstDigit(raw, '8')
			return ValidatedArgs[args]{
				Value: args{Items: []int{1}},
				Raw:   []byte(`{"items":[1]}`),
			}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
			policyViewID = req.Context.Metadata.ViewID
			mutateFirstDigit(req.Input.ArgsJSON, '9')
			req.Args.Items[0] = 9
			obj := findSchemaObject(req.Manifest.Parameters)
			if obj != nil {
				if props, ok := obj["properties"].(map[string]any); ok {
					props["items"] = "mutated_nested"
				}
			}
			return AllowDecision()
		},
		Handler: func(
			_ context.Context,
			call TypedCallContext[NoSubject, NoScope],
			_ *RunEnv,
			a ValidatedArgs[args],
		) (ToolResult[result, struct{}], error) {
			handlerViewID = call.Metadata.ViewID
			handlerFirst = a.Value.Items[0]
			return NewToolResult[result, struct{}](result{First: a.Value.Items[0]}), nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	input := ToolInput{ArgsJSON: []byte(`{"items":[1]}`)}

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "typed_copy",
		Input:    input,
		CallContext: CallContext{
			Metadata: CallMetadata{ViewID: "spoofed"},
		},
	}, func(Chunk) error { return nil })
	manifest := tool.Manifest()
	obj := findSchemaObject(manifest.Parameters)
	require.NotNil(t, obj)
	props, ok := obj["properties"].(map[string]any)
	require.True(t, ok)

	// Assert.
	require.NoError(t, err)
	assert.Empty(t, policyViewID)
	assert.Empty(t, handlerViewID)
	assert.Equal(t, 1, handlerFirst)
	assert.JSONEq(t, `{"items":[1]}`, string(input.ArgsJSON))
	assert.NotEqual(t, "mutated_nested", props["items"])
}

func TestTask31ObserverHooks_ReceiveDefensiveToolCallCopies(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		N int `json:"n"`
	}

	// Arrange.
	var handlerSaw int
	mutateInput := func(in ToolInput) {
		if len(in.ArgsJSON) > 5 {
			in.ArgsJSON[5] = '9'
		}
	}
	tool, err := NewTool("hook_copy", "Hook copy", func(_ context.Context, _ *RunEnv, a args) (result, error) {
		handlerSaw = a.N
		return result(a), nil
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithOnBeforeExecute(func(_ context.Context, call ToolCall) {
			mutateInput(call.Input)
		}),
		WithOnAfterExecute(func(_ context.Context, call ToolCall, _ ExecutionSummary, _ time.Duration) {
			mutateInput(call.Input)
		}),
	).Add(tool).Build()
	require.NoError(t, err)
	input := ToolInput{ArgsJSON: []byte(`{"n":1}`)}

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "hook_copy",
		Input:    input,
	}, func(Chunk) error { return nil })

	// Assert.
	require.NoError(t, err)
	assert.Equal(t, 1, handlerSaw)
	assert.Equal(t, []byte(`{"n":1}`), input.ArgsJSON)
}

func TestTask31RegistryView_SnapshotContractAndExecutionPolicy(t *testing.T) {
	t.Parallel()

	// Arrange.
	toolA := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	toolB := newMiddlewareMinTool(
		"b",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	var policySawView bool
	var sessionPolicySawView bool
	reg, err := NewRegistry(toolA, toolB)
	require.NoError(t, err)
	view, err := reg.View(RegistryViewSpec{
		ToolNames:         []string{"a"},
		RequiredToolNames: []string{"a"},
		Reason:            "contract test",
		Owner:             "task31",
		PolicyID:          "task31-policy",
		Policy: PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
			ok := req.View.ID != "" &&
				req.CallContext.Metadata.ViewID == req.View.ID &&
				req.View.Reason == "contract test" &&
				req.View.Owner == "task31" &&
				assert.ObjectsAreEqual([]string{"a"}, req.View.ToolNames)
			policySawView = policySawView || ok
			sessionPolicySawView = sessionPolicySawView || (ok && req.Input.CallID == "session-call")
			return AllowDecision()
		}),
	})
	require.NoError(t, err)

	// Act.
	snapshot := view.Snapshot()
	manifestSet, err := view.ManifestSet()
	require.NoError(t, err)
	err = view.Execute(context.Background(), ToolCall{
		ToolName: "a",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	require.NoError(t, err)
	sess, err := view.NewSession()
	require.NoError(t, err)
	sessionOutcome, sessionErr := sess.RunCall(context.Background(), ToolCall{
		ToolName: "a",
		Input:    ToolInput{CallID: "session-call", ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})
	var restoredPolicySawID bool
	restored, err := reg.RestoreView(snapshot, PolicyFunc(func(_ context.Context, req PolicyRequest) Decision {
		restoredPolicySawID = req.View.ID == snapshot.ID &&
			req.CallContext.Metadata.ViewID == snapshot.ID
		return AllowDecision()
	}), "task31-policy")
	require.NoError(t, err)
	err = restored.Execute(context.Background(), ToolCall{
		ToolName: "a",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })
	staleDigest := snapshot
	staleDigest.ManifestDigest = "stale"
	_, staleDigestErr := reg.RestoreView(staleDigest, nil)
	staleID := snapshot
	staleID.ID = "restored-view-id"
	_, staleIDErr := reg.RestoreView(staleID, nil)
	_, stalePolicyErr := reg.RestoreView(snapshot, PolicyFunc(func(context.Context, PolicyRequest) Decision {
		return AllowDecision()
	}), "wrong-policy")
	_, missingRequiredErr := reg.View(RegistryViewSpec{
		ToolNames:         []string{"a"},
		RequiredToolNames: []string{"b"},
	})

	// Assert.
	require.NoError(t, err)
	require.NoError(t, sessionErr)
	require.Nil(t, sessionOutcome.ExecutionError)
	assert.NotEmpty(t, snapshot.ID)
	assert.Equal(t, []string{"a"}, snapshot.ToolNames)
	assert.Equal(t, []string{"a"}, snapshot.RequiredToolNames)
	assert.NotEmpty(t, snapshot.ManifestDigest)
	assert.NotEmpty(t, snapshot.PolicyDigest)
	assert.Equal(t, "contract test", snapshot.Reason)
	assert.Equal(t, "task31", snapshot.Owner)
	require.NoError(t, ValidateManifestContract(manifestSet, []string{"a"}))
	require.NoError(t, view.ValidateManifestContract(nil))
	require.Error(t, view.ValidateManifestContract([]string{"b"}))
	assert.True(t, policySawView)
	assert.True(t, sessionPolicySawView)
	assert.True(t, restoredPolicySawID)
	require.Error(t, staleDigestErr)
	require.Error(t, staleIDErr)
	require.Error(t, stalePolicyErr)
	require.Error(t, missingRequiredErr)

	// Act.
	err = view.Execute(context.Background(), ToolCall{
		ToolName: "b",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeCapabilityDenied, te.Code)
}

func TestTask31RunCall_CapabilityDeniedIsInfrastructureError(t *testing.T) {
	t.Parallel()

	// Arrange.
	toolA := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	toolB := newMiddlewareMinTool(
		"b",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistry(toolA, toolB)
	require.NoError(t, err)
	view, err := reg.View(RegistryViewSpec{ToolNames: []string{"a"}})
	require.NoError(t, err)
	sess, err := view.NewSession()
	require.NoError(t, err)

	// Act.
	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "b",
		Input:    ToolInput{ArgsJSON: []byte(`{}`)},
		Env:      NewRunEnv(sess),
	})

	// Assert.
	require.Error(t, err)
	te, ok := AsToolError(err)
	require.True(t, ok)
	assert.Equal(t, CodeCapabilityDenied, te.Code)
	assert.Equal(t, OutcomeInfrastructureError, outcome.Status)
	assert.Nil(t, outcome.ExecutionError)
}

func TestTask31TypedToolResult_EmptyAndNoopStatus(t *testing.T) {
	t.Parallel()
	type args struct {
		Mode string `json:"mode"`
	}
	type result struct {
		OK bool `json:"ok"`
	}

	// Arrange.
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, result, struct{}]{
		Name:        "status",
		Description: "Status",
		Handler: func(
			_ context.Context,
			_ TypedCallContext[NoSubject, NoScope],
			_ *RunEnv,
			a ValidatedArgs[args],
		) (ToolResult[result, struct{}], error) {
			switch a.Value.Mode {
			case "empty":
				return NewEmptyToolResult[result, struct{}](), nil
			case "noop":
				return NewNoopToolResult[result, struct{}](), nil
			default:
				return NewToolResult[result, struct{}](result{OK: true}), nil
			}
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	// Act.
	empty, emptyErr := sess.RunCall(context.Background(), ToolCall{
		ToolName: "status",
		Input:    ToolInput{ArgsJSON: []byte(`{"mode":"empty"}`)},
		Env:      NewRunEnv(sess),
	})
	noop, noopErr := sess.RunCall(context.Background(), ToolCall{
		ToolName: "status",
		Input:    ToolInput{ArgsJSON: []byte(`{"mode":"noop"}`)},
		Env:      NewRunEnv(sess),
	})

	// Assert.
	require.NoError(t, emptyErr)
	require.NoError(t, noopErr)
	assert.Equal(t, OutcomeEmptySuccess, empty.Status)
	assert.True(t, empty.EmptyResult)
	assert.False(t, empty.Noop)
	assert.Equal(t, OutcomeNoopSuccess, noop.Status)
	assert.False(t, noop.EmptyResult)
	assert.True(t, noop.Noop)
}
