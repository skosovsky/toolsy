package toolsy

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compositionProfile(t *testing.T, kind string) ExecutionProfile {
	t.Helper()
	if kind == "cache" {
		return mustResultCache(t, constantPartition)
	}
	profile, err := NewOperationProfile(
		NewMemoryOperationStore(),
		func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{Namespace: "composition", Scope: "tenant", Subject: "host", OperationID: "intent",
				AttemptID: call.Input.CallID, PolicyFingerprint: "current", CanonicalDigest: "host dependency",
				CanonicalRules: "json", DisplayJSON: []byte(`{}`)}, nil
		},
		JSONResultCodec[string, string]{},
		"host",
		time.Now,
		time.Minute,
		0,
	)
	require.NoError(t, err)
	return profile
}

func compositionExecutor(
	t *testing.T,
	tool Tool,
	profile ExecutionProfile,
	entry string,
) func(context.Context, ToolCall, func(Chunk) error) error {
	t.Helper()
	reg, err := NewRegistryBuilder(WithExecutionProfile(profile)).Add(tool).Build()
	require.NoError(t, err)
	if entry == "direct" {
		return func(ctx context.Context, call ToolCall, yield func(Chunk) error) error {
			return tool.Execute(
				ctx,
				NewRunEnv(nil, WithRunCallContext(call.CallContext), WithRunExecutionProfile(profile)),
				call.Input,
				yield,
			)
		}
	}
	if entry == "session" {
		session, err := NewSession(reg)
		require.NoError(t, err)
		return session.Execute
	}
	return reg.Execute
}

func TestPolicyCompositionPreparesBeforeProfileAndReauthorizesReplay(t *testing.T) {
	for _, kind := range []string{"cache", "operation"} {
		for _, entry := range []string{"direct", "registry", "session"} {
			t.Run(kind+"/"+entry, func(t *testing.T) {
				// Arrange: inner binder changes outer canonical args; inner ACL can be revoked.
				type args struct {
					Value string `json:"value"`
				}
				outerBinds, innerBinds, policies, calls, snapshots := 0, 0, 0, 0, 0
				revoked := false
				base, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, string, string]{
					Name:        "composed",
					Description: "Composed",
					Options:     []ToolOption{WithIdempotent()},
					ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
						innerBinds++
						return ValidatedArgs[args]{Value: args{Value: "inner"}, Raw: []byte(`{"value":"inner"}`)}, nil
					},
					Policy: func(_ context.Context, req TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
						policies++
						assert.Equal(t, "inner", req.Args.Value)
						if revoked {
							return DenyDecision("revoked")
						}
						return AllowDecision()
					},
					Handler: func(_ context.Context, _ TypedCallContext[NoSubject, NoScope], _ *RunEnv, bound ValidatedArgs[args]) (ToolResult[string, string], error) {
						calls++
						assert.Equal(t, "inner", bound.Value.Value)
						return NewToolResult[string, string](bound.Value.Value), nil
					},
				})
				require.NoError(t, err)
				tool, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, args]{Tool: base,
					ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
						outerBinds++
						return ValidatedArgs[args]{Value: args{Value: "outer"}, Raw: []byte(`{"value":"outer"}`)}, nil
					},
				})
				require.NoError(t, err)
				profile := compositionProfile(t, kind)
				spy := executionProfileFunc(
					func(ctx context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
						snapshots++
						assert.JSONEq(t, `{"value":"inner"}`, string(call.Input.ArgsJSON))
						return profile.ExecutePrepared(ctx, call, invoke, yield)
					},
				)
				execute := compositionExecutor(t, tool, spy, entry)
				call := ToolCall{
					ToolName: "composed",
					Input:    ToolInput{CallID: "first", ArgsJSON: []byte(`{"value":"model"}`)},
				}
				// Act: fill, replay, revoke the inner ACL, attempt replay again.
				require.NoError(t, execute(context.Background(), call, func(Chunk) error { return nil }))
				call.Input.CallID = "replay"
				require.NoError(t, execute(context.Background(), call, func(Chunk) error { return nil }))
				revoked = true
				leaked := 0
				err = execute(context.Background(), call, func(Chunk) error { leaked++; return nil })
				// Assert: every binder/policy runs once per attempt, including replay/denial.
				require.ErrorIs(t, err, ErrPolicyDenied)
				assert.Equal(t, 3, outerBinds)
				assert.Equal(t, 3, innerBinds)
				assert.Equal(t, 3, policies)
				assert.Equal(t, 2, snapshots)
				assert.Equal(t, 1, calls)
				assert.Zero(t, leaked)
			})
		}
	}
}

func TestPolicyCompositionAppliesCurrentEnvelopeToReplay(t *testing.T) {
	for _, kind := range []string{"cache", "operation"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: the same stored outcome survives a stricter delivery wrapper.
			calls := 0
			base, err := NewTool("delivery", "Delivery", func(context.Context, *RunEnv, struct{}) (string, error) {
				calls++
				return "private", nil
			}, WithIdempotent())
			require.NoError(t, err)
			profile := compositionProfile(t, kind)
			wrap := func(audience ToolAudience, delivery ToolDeliveryClass, classification string) Tool {
				tool, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, struct{}]{
					Tool:             base,
					Audience:         audience,
					DeliveryClass:    delivery,
					EnvelopeMetadata: map[string]any{"classification": classification},
					ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
						return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
					},
				})
				require.NoError(t, err)
				return tool
			}
			call := ToolCall{ToolName: "delivery", Input: ToolInput{CallID: "first", ArgsJSON: []byte(`{}`)}}
			first := compositionExecutor(t, wrap(AudienceModel, DeliveryClassStructured, "public"), profile, "registry")
			require.NoError(t, first(context.Background(), call, func(Chunk) error { return nil }))
			// Act: replay through the current internal delivery contract.
			second := compositionExecutor(t, wrap(AudienceInternal, DeliveryClassText, "private"), profile, "registry")
			call.Input.CallID = "replay"
			var received Chunk
			require.NoError(t, second(context.Background(), call, func(c Chunk) error { received = c; return nil }))
			// Assert: stale stored delivery settings cannot override current restrictions.
			assert.Equal(t, 1, calls)
			assert.Equal(t, AudienceInternal, received.ToolEnvelope().Audience)
			assert.Equal(t, DeliveryClassText, received.ToolEnvelope().DeliveryClass)
			assert.Equal(t, "private", received.ToolEnvelope().Metadata["classification"])
		})
	}
}

func TestPolicyCompositionOuterPolicyChecksFinalHandlerArgs(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(strconv.FormatBool(protected), func(t *testing.T) {
			// Arrange: outer ACL permits only the pre-normalization value.
			type args struct {
				Value string `json:"value"`
			}
			calls, policies := 0, 0
			base, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, string, string]{
				Name:        "denied",
				Description: "Denied",
				Options:     []ToolOption{WithIdempotent()},
				ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
					return ValidatedArgs[args]{
						Value: args{Value: "different"},
						Raw:   []byte(`{"value":"different"}`),
					}, nil
				},
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[args]) (ToolResult[string, string], error) {
					calls++
					return NewToolResult[string, string]("forbidden"), nil
				},
			})
			require.NoError(t, err)
			tool, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, args]{Tool: base,
				ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
					return ValidatedArgs[args]{Value: args{Value: "approved"}, Raw: []byte(`{"value":"approved"}`)}, nil
				},
				Policy: func(_ context.Context, req TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
					policies++
					assert.Equal(t, "different", req.Args.Value)
					assert.JSONEq(t, `{"value":"different"}`, string(req.BoundArgs.Raw))
					if req.Args.Value == "approved" {
						return AllowDecision()
					}
					return DenyDecision("changed action")
				},
			})
			require.NoError(t, err)
			var profile ExecutionProfile
			if protected {
				profile = compositionProfile(t, "cache")
			}
			execute := compositionExecutor(t, tool, profile, "direct")
			// Act.
			err = execute(
				context.Background(),
				ToolCall{ToolName: "denied", Input: ToolInput{ArgsJSON: []byte(`{"value":"raw"}`)}},
				func(Chunk) error { t.Fatal("denied data delivered"); return nil },
			)
			// Assert: authorization concerns the actual handler values, even without a profile.
			require.ErrorIs(t, err, ErrPolicyDenied)
			assert.Equal(t, 1, policies)
			assert.Zero(t, calls)
		})
	}
}

func TestPolicyCompositionReplayNeverExpandsAudience(t *testing.T) {
	for _, kind := range []string{"cache", "operation"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: cache/journal captured an internal-only outcome.
			base, err := NewTool(
				"private",
				"Private",
				func(context.Context, *RunEnv, struct{}) (string, error) { return "private", nil },
				WithIdempotent(),
			)
			require.NoError(t, err)
			wrap := func(audience ToolAudience) Tool {
				tool, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, struct{}]{Tool: base, Audience: audience,
					ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[struct{}], error) {
						return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
					},
				})
				require.NoError(t, err)
				return tool
			}
			profile := compositionProfile(t, kind)
			call := ToolCall{ToolName: "private", Input: ToolInput{CallID: "first", ArgsJSON: []byte(`{}`)}}
			require.NoError(
				t,
				compositionExecutor(
					t,
					wrap(AudienceInternal),
					profile,
					"registry",
				)(
					context.Background(),
					call,
					func(Chunk) error { return nil },
				),
			)
			// Act: change current wrapper to a public delivery target.
			call.Input.CallID = "replay"
			var received Chunk
			require.NoError(
				t,
				compositionExecutor(
					t,
					wrap(AudienceModel),
					profile,
					"registry",
				)(
					context.Background(),
					call,
					func(c Chunk) error { received = c; return nil },
				),
			)
			// Assert: stored private data stays private under the broader wrapper.
			assert.Equal(t, expectedReplaySource(kind), received.ToolEnvelope().Metadata[ReplaySourceMetadata])
			assert.Equal(t, AudienceInternal, received.ToolEnvelope().Audience)
		})
	}
}

func TestPolicyPreparationDoesNotLeakIntoNestedExecutor(t *testing.T) {
	// Arrange: unrelated child has a different schema and delivery audience.
	childCalls, parentPolicies := 0, 0
	child, err := NewTool("child", "Child", func(context.Context, *RunEnv, struct{}) (string, error) {
		childCalls++
		return "child result", nil
	}, WithIdempotent())
	require.NoError(t, err)
	childReg, err := NewRegistry(child)
	require.NoError(t, err)
	type args struct {
		Action string `json:"action"`
	}
	var childAudience ToolAudience
	base, err := NewTool("parent", "Parent", func(ctx context.Context, env *RunEnv, _ args) (string, error) {
		childErr := childReg.Execute(
			ctx,
			ToolCall{ToolName: "child", Env: env, Input: ToolInput{ArgsJSON: []byte(`{}`)}},
			func(c Chunk) error {
				childAudience = c.ToolEnvelope().Audience
				return nil
			},
		)
		return "parent result", childErr
	})
	require.NoError(t, err)
	parent, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, args]{Tool: base, Audience: AudienceInternal,
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{Value: args{Action: "parent"}, Raw: []byte(`{"action":"parent"}`)}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
			parentPolicies++
			assert.Equal(t, "parent", req.Manifest.Name)
			assert.Equal(t, "parent", req.Args.Action)
			return AllowDecision()
		},
	})
	require.NoError(t, err)
	execute := compositionExecutor(t, parent, compositionProfile(t, "cache"), "session")
	// Act: parent passes its runtime env to the explicitly supplied child executor.
	err = execute(
		context.Background(),
		ToolCall{ToolName: "parent", Input: ToolInput{ArgsJSON: []byte(`{"action":"raw"}`)}},
		func(Chunk) error { return nil },
	)
	// Assert: parent schema/policy/audience do not become the child's contract.
	require.NoError(t, err)
	assert.Equal(t, 1, parentPolicies)
	assert.Equal(t, 1, childCalls)
	assert.Equal(t, AudienceModel, childAudience)
}
