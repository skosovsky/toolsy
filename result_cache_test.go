package toolsy

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultCacheAuthorizesEveryReplay(t *testing.T) {
	for _, policyWrapper := range []bool{false, true} {
		t.Run(strconv.FormatBool(policyWrapper), func(t *testing.T) {
			// Arrange: a protected tool caches Alice's internal result.
			var calls, policies atomic.Int32
			var revoked atomic.Bool
			policy := func(_ context.Context, req TypedPolicyRequest[string, string, struct{}]) Decision {
				policies.Add(1)
				if req.Context.Subject != "alice" || revoked.Load() {
					return DenyDecision("not allowed")
				}
				return AllowDecision()
			}
			tool, err := NewTypedTool(TypedToolSpec[string, string, struct{}, string, string]{
				Name:        "secret",
				Description: "Secret",
				Policy:      policy,
				Options:     []ToolOption{WithIdempotent()},
				Handler: func(_ context.Context, call TypedCallContext[string, string], _ *RunEnv, _ ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
					calls.Add(1)
					result := NewToolResult[string, string]("secret-" + call.Scope)
					result.Audience = AudienceInternal
					result.EnvelopeMetadata = map[string]any{"sensitive": true}
					return result, nil
				},
			})
			require.NoError(t, err)
			if policyWrapper {
				base, baseErr := NewTool(
					"secret",
					"Secret",
					func(_ context.Context, env *RunEnv, _ struct{}) (string, error) {
						calls.Add(1)
						return "secret-" + env.CallContext().Scope.(string), nil
					},
					WithIdempotent(),
				)
				require.NoError(t, baseErr)
				tool, err = NewPolicyTool(
					ToolPolicySpec[string, string, struct{}]{
						Tool:     base,
						Policy:   policy,
						Audience: AudienceInternal,
						EnvelopeMetadata: map[string]any{
							"sensitive": true,
						},
						ArgsBinder: func(_ context.Context, _ ArgsBindRequest) (ValidatedArgs[struct{}], error) {
							return ValidatedArgs[struct{}]{Raw: []byte(`{}`)}, nil
						},
					},
				)
				require.NoError(t, err)
			}
			cache := mustResultCache(t, func(_ context.Context, call PreparedCall) (string, error) {
				return call.Context.Subject.(string) + ":" + call.Context.Scope.(string), nil
			})
			reg, err := NewRegistryBuilder(WithExecutionProfile(cache)).Add(tool).Build()
			require.NoError(t, err)
			call := ToolCall{
				ToolName:    "secret",
				Input:       ToolInput{CallID: "first", ArgsJSON: []byte(`{}`)},
				CallContext: NewCallContext("alice", "tenant-a"),
			}
			first := runCacheCall(t, reg, call)
			require.Equal(t, AudienceInternal, first.ToolEnvelope().Audience)

			// Act: allowed replay, another principal, and then revoked policy.
			call.Input.CallID = "second"
			replay := runCacheCall(t, reg, call)
			bob := call
			bob.CallContext = NewCallContext("bob", "tenant-b")
			var leaked int
			err = reg.Execute(context.Background(), bob, func(Chunk) error { leaked++; return nil })
			require.Error(t, err)
			revoked.Store(true)
			err = reg.Execute(context.Background(), call, func(Chunk) error { leaked++; return nil })

			// Assert: typed policy/binding run on every call and replay preserves privacy.
			require.Error(t, err)
			assert.Equal(t, 0, leaked)
			assert.Equal(t, int32(1), calls.Load())
			assert.Equal(t, int32(4), policies.Load())
			assert.Equal(t, first.Data, replay.Data)
			assert.Equal(t, AudienceInternal, replay.ToolEnvelope().Audience)
			assert.Equal(t, true, replay.ToolEnvelope().Metadata["sensitive"])
			assert.Equal(t, ReplaySourceCache, replay.ToolEnvelope().Metadata[ReplaySourceMetadata])
			assert.Equal(t, "second", replay.CallID)
		})
	}
}

func TestResultCachePreservesCompleteTypedOutcome(t *testing.T) {
	// Arrange: non-JSON wire data, typed result/effect and a control declaration.
	var calls int
	tool, err := NewTypedTool(
		TypedToolSpec[string, string, struct{}, string, string]{
			Name:        "full",
			Description: "Full",
			Options:     []ToolOption{WithIdempotent()},
			Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				calls++
				result := NewToolResult[string, string]("typed")
				result.Raw, result.RawMimeType = []byte("binary"), MimeTypeOctetStream
				result.Audience, result.DeliveryClass = AudienceUser, DeliveryClassStructured
				result.Effects, result.Controls = []string{
					"effect",
				}, []ControlSignal{
					&UIActionSignal{Action: "show", PayloadJSON: []byte(`{}`)},
				}
				result.EnvelopeMetadata = map[string]any{"classification": "private"}
				return result, nil
			},
		},
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(mustResultCache(t, constantPartition))).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{
		ToolName:    "full",
		Input:       ToolInput{ArgsJSON: []byte(`{}`)},
		CallContext: NewCallContext("alice", "scope"),
	}
	first := runCacheCall(t, reg, call)
	// Act: replay.
	second := runCacheCall(t, reg, call)
	// Assert: no type erasure or audience broadening.
	assert.Equal(t, 1, calls)
	assert.Equal(t, first.Data, second.Data)
	assert.Equal(t, MimeTypeOctetStream, second.MimeType)
	assert.Equal(t, "typed", second.TypedResult)
	assert.Equal(t, first.Effects, second.Effects)
	assert.Equal(t, first.Controls, second.Controls)
	assert.Equal(t, AudienceUser, second.ToolEnvelope().Audience)
	assert.Equal(t, DeliveryClassStructured, second.ToolEnvelope().DeliveryClass)
	assert.Equal(t, "private", second.ToolEnvelope().Metadata["classification"])
}

func TestResultCacheUsesCanonicalArgsAndAttachments(t *testing.T) {
	// Arrange: a binder maps raw inputs to the same normalized argument.
	type args struct {
		N int `json:"n"`
	}
	var binds, calls int
	tool, err := NewTypedTool(
		TypedToolSpec[string, string, args, string, string]{
			Name:        "canonical",
			Description: "Canonical",
			Options:     []ToolOption{WithIdempotent()},
			ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
				binds++
				return ValidatedArgs[args]{Value: args{N: 1}, Raw: []byte(`{"n":1}`)}, nil
			},
			Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[args]) (ToolResult[string, string], error) {
				calls++
				return NewToolResult[string, string]("ok"), nil
			},
		},
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(mustResultCache(t, constantPartition))).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{
		ToolName:    "canonical",
		Input:       ToolInput{ArgsJSON: []byte(`{"n":9}`)},
		CallContext: NewCallContext("alice", "scope"),
	}
	// Act: semantically equal inputs and then changed attachment data.
	runCacheCall(t, reg, call)
	call.Input.ArgsJSON = []byte(`{"n":10}`)
	runCacheCall(t, reg, call)
	call.Input.Attachments = []Attachment{{MimeType: MimeTypeText, Data: []byte("changed")}}
	runCacheCall(t, reg, call)
	// Assert: binder executes once per attempt, canonical equal inputs replay.
	assert.Equal(t, 3, binds)
	assert.Equal(t, 2, calls)
}

func TestResultCacheFailsClosedOnMissingPartition(t *testing.T) {
	// Arrange.
	var calls int
	tool, err := NewTool(
		"cache",
		"Cache",
		func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "ok", nil },
		WithIdempotent(),
	)
	require.NoError(t, err)
	cache := mustResultCache(t, func(context.Context, PreparedCall) (string, error) { return "", nil })
	reg, err := NewRegistryBuilder(WithExecutionProfile(cache)).Add(tool).Build()
	require.NoError(t, err)
	// Act.
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "cache", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { return nil },
	)
	// Assert.
	require.Error(t, err)
	assert.Zero(t, calls)
}

func TestResultCacheAsyncPolicyAndPrivacy(t *testing.T) {
	// Arrange: the prepared profile is executed inside background work.
	var calls, policies atomic.Int32
	tool, err := NewTypedTool(
		TypedToolSpec[string, string, struct{}, string, string]{
			Name:        "async_secret",
			Description: "Async secret",
			Options:     []ToolOption{WithIdempotent()},
			Policy: func(_ context.Context, req TypedPolicyRequest[string, string, struct{}]) Decision {
				policies.Add(1)
				if req.Context.Subject != "alice" {
					return DenyDecision("denied")
				}
				return AllowDecision()
			},
			Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				calls.Add(1)
				result := NewToolResult[string, string]("secret")
				result.Audience = AudienceInternal
				return result, nil
			},
		},
	)
	require.NoError(t, err)
	type completion struct {
		chunks []Chunk
		err    error
	}
	done := make(chan completion, 1)
	reg, err := NewRegistryBuilder(
		WithExecutionProfile(mustResultCache(t, constantPartition)),
	).Add(AsAsyncTool(tool, WithOnComplete(func(_ context.Context, _ string, chunks []Chunk, err error) { done <- completion{chunks, err} }))).
		Build()
	require.NoError(t, err)
	// Act: allowed fill/replay, then denied principal.
	for _, subject := range []string{"alice", "alice", "bob"} {
		call := ToolCall{
			ToolName:    "async_secret",
			Input:       ToolInput{ArgsJSON: []byte(`{}`)},
			CallContext: NewCallContext(subject, "scope"),
		}
		require.NoError(t, reg.Execute(context.Background(), call, func(Chunk) error { return nil }))
		select {
		case result := <-done:
			if subject == "bob" {
				require.Error(t, result.err)
				assert.Empty(t, result.chunks)
			} else {
				require.NoError(t, result.err)
				require.Len(t, result.chunks, 1)
				assert.Equal(t, AudienceInternal, result.chunks[0].ToolEnvelope().Audience)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("background completion not delivered")
		}
	}
	// Assert.
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, int32(3), policies.Load())
}

func TestResultCacheDoesNotCompleteFailedProducer(t *testing.T) {
	// Arrange: producer fails after yielding a candidate.
	var calls int
	tool, err := NewStreamTool(
		"broken",
		"Broken",
		func(_ context.Context, _ *RunEnv, _ struct{}, yield func(Chunk) error) error {
			calls++
			if err := yield(
				Chunk{Event: EventResult, Data: []byte(`"candidate"`), MimeType: MimeTypeJSON},
			); err != nil {
				return err
			}
			return errors.New("late producer failure")
		},
		WithIdempotent(),
		WithIndependentStream(),
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(mustResultCache(t, constantPartition))).Add(tool).Build()
	require.NoError(t, err)
	// Act.
	var delivered int
	for range 2 {
		err = reg.Execute(
			context.Background(),
			ToolCall{ToolName: "broken", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
			func(Chunk) error { delivered++; return nil },
		)
		require.Error(t, err)
	}
	// Assert: no success or poisoned cache; caller retries here deliberately for the test.
	assert.Zero(t, delivered)
	assert.Equal(t, 2, calls)
}

func mustResultCache(t *testing.T, partition CachePartition) *ResultCache {
	t.Helper()
	cache, err := NewResultCache(
		NewMemoryResultCacheStore(),
		allowTestCacheReuse,
		partition,
		JSONResultCodec[string, string]{},
		0,
	)
	require.NoError(t, err)
	return cache
}

func constantPartition(context.Context, PreparedCall) (string, error) {
	return "trusted-test-partition", nil
}

func TestResultCacheIsolatesAllowedScopes(t *testing.T) {
	// Arrange: both scopes are authorized; their data still cannot collide.
	var calls int
	tool, err := NewTypedTool(
		TypedToolSpec[string, string, struct{}, string, string]{
			Name:        "scope",
			Description: "Scope",
			Options:     []ToolOption{WithIdempotent()},
			Handler: func(_ context.Context, c TypedCallContext[string, string], _ *RunEnv, _ ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				calls++
				return NewToolResult[string, string](c.Scope), nil
			},
		},
	)
	require.NoError(t, err)
	cache := mustResultCache(
		t,
		func(_ context.Context, call PreparedCall) (string, error) { return call.Context.Scope.(string), nil },
	)
	reg, err := NewRegistryBuilder(WithExecutionProfile(cache)).Add(tool).Build()
	require.NoError(t, err)
	// Act/Assert: each scope fills and then replays its own result.
	for _, scope := range []string{"a", "b", "a", "b"} {
		result := runCacheCall(
			t,
			reg,
			ToolCall{
				ToolName:    "scope",
				Input:       ToolInput{ArgsJSON: []byte(`{}`)},
				CallContext: NewCallContext("alice", scope),
			},
		)
		assert.Equal(t, scope, result.TypedResult)
	}
	assert.Equal(t, 2, calls)
}

func TestResultCacheUnsupportedCodecFailsExplicitly(t *testing.T) {
	// Arrange: the declared codec type differs from the tool result.
	var calls int
	tool, err := NewTool(
		"unsupported",
		"Unsupported",
		func(context.Context, *RunEnv, struct{}) (string, error) { calls++; return "private", nil },
		WithIdempotent(),
	)
	require.NoError(t, err)
	cache, err := NewResultCache(
		NewMemoryResultCacheStore(),
		allowTestCacheReuse,
		constantPartition,
		JSONResultCodec[int, string]{},
		0,
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(cache)).Add(tool).Build()
	require.NoError(t, err)
	var delivered int
	// Act.
	err = reg.Execute(
		context.Background(),
		ToolCall{ToolName: "unsupported", Input: ToolInput{ArgsJSON: []byte(`{}`)}},
		func(Chunk) error { delivered++; return nil },
	)
	// Assert: no corrupted replay or automatic retry, even though handler ran.
	require.Error(t, err)
	var te *ToolError
	require.ErrorAs(t, err, &te)
	assert.False(t, te.Retryable)
	assert.Equal(t, 1, calls)
	assert.Zero(t, delivered)
}

func TestResultCodecPreservesTypedNil(t *testing.T) {
	// Arrange: interface values containing typed nil pointers are not untyped nil.
	var value *string
	chunk := Chunk{Event: EventResult, Data: []byte(`null`), MimeType: MimeTypeJSON, TypedResult: value}
	chunk.Envelope = NewResultEnvelope(
		value,
		chunk.Data,
		chunk.MimeType,
		DeliveryClassStructured,
		AudienceInternal,
		nil,
	)
	codec := JSONResultCodec[*string, string]{}
	// Act.
	raw, err := codec.EncodeResult(chunk)
	require.NoError(t, err)
	decoded, err := codec.DecodeResult(raw)
	// Assert.
	require.NoError(t, err)
	_, ok := decoded.TypedResult.(*string)
	assert.True(t, ok)
	_, ok = decoded.ToolEnvelope().Result.(*string)
	assert.True(t, ok)
}

func TestResultCacheReplayDoesNotReapplyHostEffects(t *testing.T) {
	// Arrange: the host reducer honors the replay delivery marker.
	var applied int
	tool, err := NewTypedTool(
		TypedToolSpec[string, string, struct{}, string, string]{
			Name:        "effect",
			Description: "Effect",
			Options:     []ToolOption{WithIdempotent()},
			Handler: func(context.Context, TypedCallContext[string, string], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[string, string], error) {
				result := NewToolResult[string, string]("ok")
				result.Effects = []string{"change"}
				return result, nil
			},
		},
	)
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(WithExecutionProfile(mustResultCache(t, constantPartition))).Add(tool).Build()
	require.NoError(t, err)
	// Act: original delivery and cached declaration replay.
	for range 2 {
		require.NoError(
			t,
			reg.Execute(
				context.Background(),
				ToolCall{
					ToolName:    "effect",
					Input:       ToolInput{ArgsJSON: []byte(`{}`)},
					CallContext: NewCallContext("alice", "scope"),
				},
				func(c Chunk) error {
					if c.ToolEnvelope().Metadata[ReplaySourceMetadata] == nil {
						applied += len(c.Effects)
					}
					return nil
				},
			),
		)
	}
	// Assert: one application, with declarations still preserved for introspection.
	assert.Equal(t, 1, applied)
}

func runCacheCall(t *testing.T, reg *Registry, call ToolCall) Chunk {
	t.Helper()
	var result Chunk
	require.NoError(t, reg.Execute(context.Background(), call, func(c Chunk) error { result = c; return nil }))
	return result
}

// Test fixtures explicitly accept their stable in-memory data lifetime.
func allowTestCacheReuse(context.Context, PreparedCall) (bool, error) { return true, nil }

func expectedReplaySource(kind string) string {
	if kind == "operation" {
		return ReplaySourceOperation
	}
	return ReplaySourceCache
}
