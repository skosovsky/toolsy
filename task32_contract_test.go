package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTask32TypedTool_ArgsBinderPassesValidatedBoundaryToPolicyAndHandler(t *testing.T) {
	t.Parallel()
	type subject struct {
		ID string
	}
	type scope struct {
		Workspace string
	}
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		N int `json:"n"`
	}

	// Arrange.
	var policySawMetadata, handlerSawBoundary bool
	tool, err := NewTypedTool(TypedToolSpec[subject, scope, args, result, struct{}]{
		Name:        "task32_bind",
		Description: "Task32 bind contract",
		ArgsBinder: func(_ context.Context, req ArgsBindRequest) (ValidatedArgs[args], error) {
			require.Equal(t, "task32_bind", req.Manifest.Name)
			require.JSONEq(t, `{"n":2,"unsafe":true}`, string(req.Input.ArgsJSON))
			require.Equal(t, "u1", req.CallContext.Subject.(subject).ID)
			return ValidatedArgs[args]{
				Value: args{N: 3},
				Raw:   []byte(`{"n":3}`),
				Metadata: map[string]any{
					"source":     "binder",
					"nested":     map[string]any{"safe": "yes"},
					"typedMap":   map[string]string{"safe": "yes"},
					"typedSlice": []string{"yes"},
				},
			}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[subject, scope, args]) Decision {
			nested := req.BoundArgs.Metadata["nested"].(map[string]any)
			typedMap := req.BoundArgs.Metadata["typedMap"].(map[string]string)
			typedSlice := req.BoundArgs.Metadata["typedSlice"].([]string)
			policySawMetadata = req.Args.N == 3 &&
				req.BoundArgs.Value.N == 3 &&
				string(req.BoundArgs.Raw) == `{"n":3}` &&
				req.BoundArgs.Metadata["source"] == "binder" &&
				nested["safe"] == "yes" &&
				typedMap["safe"] == "yes" &&
				typedSlice[0] == "yes" &&
				req.Context.Subject.ID == "u1" &&
				req.Context.Scope.Workspace == "w1"
			nested["safe"] = "mutated"
			typedMap["safe"] = "mutated"
			typedSlice[0] = "mutated"
			return AllowDecision()
		},
		ArgValidator: func(a args) error {
			if a.N != 3 {
				return NewValidationError("binder value was not used")
			}
			return nil
		},
		Handler: func(
			_ context.Context,
			call TypedCallContext[subject, scope],
			_ *RunEnv,
			bound ValidatedArgs[args],
		) (ToolResult[result, struct{}], error) {
			nested := bound.Metadata["nested"].(map[string]any)
			typedMap := bound.Metadata["typedMap"].(map[string]string)
			typedSlice := bound.Metadata["typedSlice"].([]string)
			handlerSawBoundary = call.Subject.ID == "u1" &&
				bound.Value.N == 3 &&
				string(bound.Raw) == `{"n":3}` &&
				bound.Metadata["source"] == "binder" &&
				nested["safe"] == "yes" &&
				typedMap["safe"] == "yes" &&
				typedSlice[0] == "yes"
			out := NewToolResult[result, struct{}](result{N: bound.Value.N})
			out.DeliveryClass = DeliveryClassStructured
			out.Audience = AudienceInternal
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
		ToolName: "task32_bind",
		Input:    ToolInput{CallID: "c1", ArgsJSON: []byte(`{"n":2,"unsafe":true}`)},
		Env:      NewRunEnv(sess),
		CallContext: NewCallContext(
			subject{ID: "u1"},
			scope{Workspace: "w1"},
		),
	})

	// Assert.
	require.NoError(t, err)
	require.Nil(t, outcome.ExecutionError)
	assert.True(t, policySawMetadata)
	assert.True(t, handlerSawBoundary)
	decoded, err := DecodeOutcomeAs[result](outcome)
	require.NoError(t, err)
	assert.Equal(t, result{N: 3}, *decoded)
	assert.Equal(t, ToolEnvelopeKindResult, outcome.Envelope.Kind)
	assert.Equal(t, DeliveryClassStructured, outcome.Envelope.DeliveryClass)
	assert.Equal(t, AudienceInternal, outcome.Envelope.Audience)
	assert.Equal(t, MimeTypeJSON, outcome.Envelope.MimeType) //nolint:testifylint // mime type, not JSON document
}

func TestTask32Session_RebindAndCheckpointOwnViewCompatibility(t *testing.T) {
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
	viewA, err := reg.View(RegistryViewSpec{ToolNames: []string{"a"}, Reason: "task32", Owner: "test"})
	require.NoError(t, err)
	viewB, err := reg.View(RegistryViewSpec{ToolNames: []string{"b"}, Reason: "task32", Owner: "test"})
	require.NoError(t, err)
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[int](codecs, "counter"))
	sess, err := viewA.NewSession(WithStateCodecRegistry(codecs))
	require.NoError(t, err)
	SetSessionState(sess, "counter", 7)

	// Act.
	checkpoint, err := sess.ExportCheckpoint()
	require.NoError(t, err)
	rawCheckpointSnapshot, err := json.Marshal(checkpoint.Snapshot)
	require.NoError(t, err)
	var unmarshaledSnapshot SessionSnapshot
	require.NoError(t, json.Unmarshal(rawCheckpointSnapshot, &unmarshaledSnapshot))
	restoredView, err := reg.RestoreView(viewA.Snapshot(), nil)
	require.NoError(t, err)
	rebindErr := restoredView.RebindSession(sess)
	_, wrongViewErr := viewB.NewSessionFromCheckpoint(checkpoint, WithStateCodecRegistry(codecs))
	wrongViewSession, err := viewB.NewSession(WithStateCodecRegistry(codecs))
	require.NoError(t, err)
	directWrongImportErr := wrongViewSession.ImportSnapshot(checkpoint.Snapshot)
	unmarshaledWrongImportErr := wrongViewSession.ImportSnapshot(unmarshaledSnapshot)
	restoredSession, restoreErr := restoredView.NewSessionFromCheckpoint(checkpoint, WithStateCodecRegistry(codecs))

	// Assert.
	require.NoError(t, rebindErr)
	assert.Equal(t, viewA.Snapshot().ID, sess.Binding().View.ID)
	got, ok := GetSessionState[int](sess, "counter")
	require.True(t, ok)
	assert.Equal(t, 7, got)
	require.Error(t, wrongViewErr)
	require.Error(t, directWrongImportErr)
	require.Error(t, unmarshaledWrongImportErr)
	require.NoError(t, restoreErr)
	restoredCounter, ok := GetSessionState[int](restoredSession, "counter")
	require.True(t, ok)
	assert.Equal(t, 7, restoredCounter)
}

func TestTask32RegistryView_PolicyIDIsPartOfSnapshotIdentity(t *testing.T) {
	t.Parallel()

	// Arrange.
	tool := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	policy := PolicyFunc(func(context.Context, PolicyRequest) Decision {
		return AllowDecision()
	})

	// Act.
	_, missingIDErr := reg.View(RegistryViewSpec{ToolNames: []string{"a"}, Policy: policy})
	_, missingPolicyErr := reg.View(RegistryViewSpec{ToolNames: []string{"a"}, PolicyID: "policy-a"})
	view, err := reg.View(RegistryViewSpec{ToolNames: []string{"a"}, Policy: policy, PolicyID: "policy-a"})
	require.NoError(t, err)
	_, wrongIDErr := reg.RestoreView(view.Snapshot(), policy, "policy-b")
	_, missingPolicyRestoreErr := reg.RestoreView(view.Snapshot(), nil, "policy-a")
	restored, restoreErr := reg.RestoreView(view.Snapshot(), policy, "policy-a")
	noPolicyView, err := reg.View(RegistryViewSpec{ToolNames: []string{"a"}})
	require.NoError(t, err)
	_, unexpectedPolicyRestoreErr := reg.RestoreView(noPolicyView.Snapshot(), policy, "policy-a")

	// Assert.
	require.Error(t, missingIDErr)
	require.Error(t, missingPolicyErr)
	require.Error(t, wrongIDErr)
	require.Error(t, missingPolicyRestoreErr)
	require.Error(t, unexpectedPolicyRestoreErr)
	require.NoError(t, restoreErr)
	assert.Equal(t, view.Snapshot().ID, restored.Snapshot().ID)
	assert.NotEmpty(t, view.Snapshot().PolicyDigest)
}

func TestTask32SessionCheckpoint_RootPolicyIDIsPartOfBinding(t *testing.T) {
	t.Parallel()

	// Arrange.
	tool := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	policy := PolicyFunc(func(context.Context, PolicyRequest) Decision {
		return AllowDecision()
	})
	regA, err := NewRegistryBuilder(WithPolicy("root-policy-a", policy)).Add(tool).Build()
	require.NoError(t, err)
	regB, err := NewRegistryBuilder(WithPolicy("root-policy-b", policy)).Add(tool).Build()
	require.NoError(t, err)
	_, missingPolicyIDErr := NewRegistryBuilder(WithPolicy("", policy)).Add(tool).Build()
	_, chainedMissingPolicyIDErr := NewRegistryBuilder(
		WithPolicy("", policy),
		WithPolicy("root-policy-c", policy),
	).Add(tool).Build()
	sess, err := NewSession(regA)
	require.NoError(t, err)
	SetSessionState(sess, "counter", 7)

	// Act.
	checkpoint, err := sess.ExportCheckpoint()
	require.NoError(t, err)
	_, restoreErr := NewSessionFromCheckpoint(regB, checkpoint)
	wrongPolicySession, err := NewSession(regB)
	require.NoError(t, err)
	importErr := wrongPolicySession.ImportSnapshot(checkpoint.Snapshot)
	rebindErr := sess.Rebind(regB)

	// Assert.
	require.Error(t, missingPolicyIDErr)
	require.Error(t, chainedMissingPolicyIDErr)
	require.Error(t, restoreErr)
	requireToolErrorCode(t, restoreErr, CodeToolsContractMissing)
	require.Error(t, importErr)
	requireToolErrorCode(t, importErr, CodeToolsContractMissing)
	require.Error(t, rebindErr)
	requireToolErrorCode(t, rebindErr, CodeToolsContractMissing)
	assert.NotEmpty(t, checkpoint.Binding.PolicyDigest)
}

func TestTask32SessionCheckpoint_RejectsIncompatibleStateSchema(t *testing.T) {
	t.Parallel()

	// Arrange.
	tool := newMiddlewareMinTool(
		"a",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	view, err := reg.View(RegistryViewSpec{ToolNames: []string{"a"}, Reason: "task32", Owner: "test"})
	require.NoError(t, err)
	intCodecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[int](intCodecs, "counter"))
	stringCodecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[string](stringCodecs, "counter"))
	sess, err := view.NewSession(WithStateCodecRegistry(intCodecs))
	require.NoError(t, err)
	SetSessionState(sess, "counter", 7)

	// Act.
	checkpoint, err := sess.ExportCheckpoint()
	require.NoError(t, err)
	_, restoreErr := view.NewSessionFromCheckpoint(checkpoint, WithStateCodecRegistry(stringCodecs))
	wrongSchemaSession, err := view.NewSession(WithStateCodecRegistry(stringCodecs))
	require.NoError(t, err)
	importErr := wrongSchemaSession.ImportSnapshot(checkpoint.Snapshot)

	// Assert.
	require.Error(t, restoreErr)
	requireToolErrorCode(t, restoreErr, CodeToolsContractMissing)
	require.Error(t, importErr)
	requireToolErrorCode(t, importErr, CodeToolsContractMissing)
}

func TestTask32SnapshotImport_RejectsNullAndMissingRequiredSlotsBySchema(t *testing.T) {
	t.Parallel()
	type payload struct {
		Name string `json:"name"`
	}

	// Arrange.
	codecs := NewStateCodecRegistry()
	require.NoError(t, RegisterJSONCodec[payload](codecs, "payload", WithStateSlotRequired()))
	sess := newTestSession(t, WithStateCodecRegistry(codecs), WithStrictStateCodecs(true))
	SetSessionState(sess, "payload", payload{Name: "keep"})
	nullSnap := mustTask32Snapshot(t, sess.Binding(), `{"payload":null}`)
	missingSnap := mustTask32Snapshot(t, sess.Binding(), `{}`)

	// Act.
	nullErr := sess.ImportSnapshot(nullSnap)
	gotAfterNull, okAfterNull := GetSessionState[payload](sess, "payload")
	missingErr := sess.ImportSnapshot(missingSnap)

	// Assert.
	require.Error(t, nullErr)
	requireToolErrorCode(t, nullErr, CodeInternal)
	require.True(t, okAfterNull)
	assert.Equal(t, payload{Name: "keep"}, gotAfterNull)
	require.Error(t, missingErr)
	requireToolErrorCode(t, missingErr, CodeInternal)
}

func TestTask32PolicyTool_PassesSanitizedRawToBaseToolWithoutHostWrapper(t *testing.T) {
	t.Parallel()
	type args struct {
		Query string `json:"query"`
	}
	type subject struct {
		Allowed bool
	}
	type scope struct {
		Tenant string
	}

	// Arrange.
	var baseSawSanitized, policySawTyped bool
	var firstChunk, secondChunk Chunk
	base := newMiddlewareMinTool(
		"generic_search",
		func(_ context.Context, _ *RunEnv, input ToolInput, yield func(Chunk) error) error {
			baseSawSanitized = assert.JSONEq(t, `{"query":"safe"}`, string(input.ArgsJSON))
			require.Len(t, input.Attachments, 1)
			assert.Equal(t, "payload", string(input.Attachments[0].Data))
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
	)
	wrapped, err := NewPolicyTool(ToolPolicySpec[subject, scope, args]{
		Tool: base,
		Requirements: ToolRequirements{
			Permissions: []Permission{"search"},
		},
		ArgsBinder: func(_ context.Context, _ ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{
				Value: args{Query: "safe"},
				Raw:   []byte(`{"query":"safe"}`),
			}, nil
		},
		Policy: func(_ context.Context, req TypedPolicyRequest[subject, scope, args]) Decision {
			policySawTyped = req.Args.Query == "safe" &&
				req.Context.Subject.Allowed &&
				req.Context.Scope.Tenant == "t1"
			return AllowDecision()
		},
		DeliveryClass: DeliveryClassStructured,
		Audience:      AudienceInternal,
		EnvelopeMetadata: map[string]any{
			"owner":      "policy-tool",
			"typedMap":   map[string]string{"safe": "yes"},
			"typedSlice": []string{"yes"},
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithRequirementsPolicy("task32-search-requirements-policy", func(
			_ context.Context,
			req RequirementsPolicyRequest[subject, scope],
		) Decision {
			if req.Context.Subject.Allowed {
				return AllowDecision()
			}
			return DenyDecision("blocked")
		}),
	).Add(wrapped).Build()
	require.NoError(t, err)
	execute := func(capture *Chunk) error {
		return reg.Execute(context.Background(), ToolCall{
			ToolName: "generic_search",
			Input: ToolInput{
				ArgsJSON:    []byte(`{"query":"unsafe","ignored":true}`),
				Attachments: []Attachment{{MimeType: MimeTypeText, Data: []byte("payload")}},
			},
			CallContext: NewCallContext(
				subject{Allowed: true},
				scope{Tenant: "t1"},
			),
		}, func(c Chunk) error {
			*capture = c
			return nil
		})
	}

	// Act.
	firstErr := execute(&firstChunk)
	firstChunk.Envelope.Metadata["typedMap"].(map[string]string)["safe"] = "mutated"
	firstChunk.Envelope.Metadata["typedSlice"].([]string)[0] = "mutated"
	secondErr := execute(&secondChunk)

	// Assert.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.True(t, policySawTyped)
	assert.True(t, baseSawSanitized)
	assert.Equal(t, []Permission{"search"}, wrapped.Manifest().Requirements.Permissions)
	assert.Equal(t, AudienceInternal, secondChunk.ToolEnvelope().Audience)
	assert.Equal(t, "policy-tool", secondChunk.ToolEnvelope().Metadata["owner"])
	assert.Equal(t, "yes", secondChunk.ToolEnvelope().Metadata["typedMap"].(map[string]string)["safe"])
	assert.Equal(t, "yes", secondChunk.ToolEnvelope().Metadata["typedSlice"].([]string)[0])
}

func TestTask32PrepareChunk_ClonesExistingEnvelopeBeforeDelivery(t *testing.T) {
	t.Parallel()

	// Arrange.
	sharedEnvelope := NewResultEnvelope(
		map[string]bool{"ok": true},
		[]byte(`{"ok":true}`),
		MimeTypeJSON,
		DeliveryClassStructured,
		AudienceInternal,
		map[string]any{
			"typedMap":   map[string]string{"safe": "yes"},
			"typedSlice": []string{"yes"},
		},
	)
	tool := newMiddlewareMinTool(
		"shared_envelope",
		func(_ context.Context, _ *RunEnv, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte(`{"ok":true}`),
				MimeType: MimeTypeJSON,
				Envelope: sharedEnvelope,
			})
		},
	)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	execute := func(capture *Chunk) error {
		return reg.Execute(context.Background(), ToolCall{
			ToolName: "shared_envelope",
			Input: ToolInput{
				ArgsJSON: []byte(`{}`),
			},
		}, func(c Chunk) error {
			*capture = c
			return nil
		})
	}

	// Act.
	var firstChunk, secondChunk Chunk
	firstErr := execute(&firstChunk)
	firstChunk.Envelope.Metadata["typedMap"].(map[string]string)["safe"] = "mutated"
	firstChunk.Envelope.Metadata["typedSlice"].([]string)[0] = "mutated"
	secondErr := execute(&secondChunk)

	// Assert.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NotNil(t, secondChunk.Envelope)
	assert.Equal(t, "yes", secondChunk.Envelope.Metadata["typedMap"].(map[string]string)["safe"])
	assert.Equal(t, "yes", secondChunk.Envelope.Metadata["typedSlice"].([]string)[0])
	assert.Equal(t, "yes", sharedEnvelope.Metadata["typedMap"].(map[string]string)["safe"])
	assert.Equal(t, "yes", sharedEnvelope.Metadata["typedSlice"].([]string)[0])
}

func TestTask32PolicyTool_EmptyBinderRawFailsClosed(t *testing.T) {
	t.Parallel()
	type args struct {
		Query string `json:"query"`
	}

	// Arrange.
	base := newMiddlewareMinTool(
		"generic_empty_raw",
		func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
			t.Fatal("base tool must not run without canonical raw args")
			return nil
		},
	)
	wrapped, err := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, args]{
		Tool: base,
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{Value: args{Query: "safe"}}, nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(wrapped)
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "generic_empty_raw",
		Input:    ToolInput{ArgsJSON: []byte(`{"query":"unsafe"}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeValidationFailed)
}

func TestTask32PolicyTool_RequiresArgsBinder(t *testing.T) {
	t.Parallel()
	type args struct {
		Query string `json:"query"`
	}

	// Arrange.
	base := newMiddlewareMinTool(
		"generic_missing_binder",
		func(context.Context, *RunEnv, ToolInput, func(Chunk) error) error {
			t.Fatal("base tool must not be wrapped without a binder")
			return nil
		},
	)

	// Act.
	_, wrapErr := NewPolicyTool(ToolPolicySpec[NoSubject, NoScope, args]{
		Tool: base,
		Policy: func(context.Context, TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
			return AllowDecision()
		},
	})
	_, fromSpecErr := NewPolicyToolFromSpec(ToolPolicyConstructorSpec[NoSubject, NoScope, args]{
		Name:          "generic_missing_binder_from_spec",
		Description:   "Generic missing binder from spec",
		RawJSONSchema: []byte(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		Handler: func(context.Context, *RunEnv, []byte, func(Chunk) error) error {
			t.Fatal("handler must not be wrapped without a binder")
			return nil
		},
		Policy: func(context.Context, TypedPolicyRequest[NoSubject, NoScope, args]) Decision {
			return AllowDecision()
		},
	})

	// Assert.
	require.Error(t, wrapErr)
	require.Error(t, fromSpecErr)
}

func TestTask32PolicyToolFromSpec_BuildsProductionGenericTool(t *testing.T) {
	t.Parallel()
	type args struct {
		Query string `json:"query"`
	}

	// Arrange.
	var handlerSawRaw bool
	tool, err := NewPolicyToolFromSpec(ToolPolicyConstructorSpec[NoSubject, NoScope, args]{
		Name:          "generic_from_spec",
		Description:   "Generic from spec",
		RawJSONSchema: []byte(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		Handler: func(_ context.Context, _ *RunEnv, raw []byte, yield func(Chunk) error) error {
			handlerSawRaw = assert.JSONEq(t, `{"query":"safe"}`, string(raw))
			return yield(Chunk{Event: EventResult, Data: []byte(`{"ok":true}`), MimeType: MimeTypeJSON})
		},
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{
				Value: args{Query: "safe"},
				Raw:   []byte(`{"query":"safe"}`),
			}, nil
		},
		Requirements: ToolRequirements{Permissions: []Permission{"search"}},
	})
	require.NoError(t, err)
	reg, err := NewRegistryBuilder(
		WithRequirementsPolicy(
			"task32-generic-requirements-policy",
			func(context.Context, RequirementsPolicyRequest[NoSubject, NoScope]) Decision {
				return AllowDecision()
			},
		),
	).Add(tool).Build()
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "generic_from_spec",
		Input:    ToolInput{ArgsJSON: []byte(`{"query":"unsafe"}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.NoError(t, err)
	assert.True(t, handlerSawRaw)
	assert.Equal(t, []Permission{"search"}, tool.Manifest().Requirements.Permissions)
}

func TestTask32ToolEnvelope_FromErrorChunkProvidesTypedClassification(t *testing.T) {
	t.Parallel()

	// Arrange.
	errChunk := NewErrorChunkFromErr(WithSafeMessage(NewValidationError("bad input", "query"), "Fix query"))

	// Act.
	prepared, err := prepareChunk(errChunk)
	require.NoError(t, err)
	envelope := prepared.ToolEnvelope()

	// Assert.
	require.Equal(t, ToolEnvelopeKindError, envelope.Kind)
	require.NotNil(t, envelope.Error)
	assert.Equal(t, CodeValidationFailed, envelope.Error.Code)
	assert.Equal(t, DeliveryClassStructured, envelope.DeliveryClass)
	assert.Equal(t, AudienceModel, envelope.Audience)
	assert.Equal(t, MimeTypeToolErrorJSON, envelope.MimeType) //nolint:testifylint // mime type, not JSON document
	assert.True(t, json.Valid(envelope.Raw))
}

func TestTask32ToolEnvelope_TextMediaTypesAreNotStructured(t *testing.T) {
	t.Parallel()

	// Arrange.
	cases := []string{
		"text/plain",
		MimeTypeText,
		"text/markdown; charset=utf-8",
	}

	for _, mimeType := range cases {
		t.Run(mimeType, func(t *testing.T) {
			t.Parallel()

			// Act.
			envelope := NewResultEnvelope(nil, []byte("hello"), mimeType, "", "", nil)

			// Assert.
			assert.Equal(t, DeliveryClassText, envelope.DeliveryClass)
		})
	}
}

func TestTask32TypedTool_BinderErrorIsStructuredValidation(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		N int `json:"n"`
	}

	// Arrange.
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, result, struct{}]{
		Name:        "task32_bind_error",
		Description: "Task32 bind error",
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{}, errors.New("unsafe args")
		},
		Handler: func(
			context.Context,
			TypedCallContext[NoSubject, NoScope],
			*RunEnv,
			ValidatedArgs[args],
		) (ToolResult[result, struct{}], error) {
			t.Fatal("handler must not run when binder fails")
			return NewToolResult[result, struct{}](result{}), nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)

	// Act.
	err = reg.Execute(context.Background(), ToolCall{
		ToolName: "task32_bind_error",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
	}, func(Chunk) error { return nil })

	// Assert.
	require.Error(t, err)
	requireToolErrorCode(t, err, CodeValidationFailed)
}

func TestTask32RunCall_HardBusinessErrorCarriesEnvelope(t *testing.T) {
	t.Parallel()
	type args struct {
		N int `json:"n"`
	}
	type result struct {
		N int `json:"n"`
	}

	// Arrange.
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, args, result, struct{}]{
		Name:        "task32_hard_error",
		Description: "Task32 hard error",
		ArgsBinder: func(context.Context, ArgsBindRequest) (ValidatedArgs[args], error) {
			return ValidatedArgs[args]{}, errors.New("unsafe args")
		},
		Handler: func(
			context.Context,
			TypedCallContext[NoSubject, NoScope],
			*RunEnv,
			ValidatedArgs[args],
		) (ToolResult[result, struct{}], error) {
			t.Fatal("handler must not run")
			return NewToolResult[result, struct{}](result{}), nil
		},
	})
	require.NoError(t, err)
	reg, err := NewRegistry(tool)
	require.NoError(t, err)
	sess, err := NewSession(reg)
	require.NoError(t, err)

	// Act.
	outcome, err := sess.RunCall(context.Background(), ToolCall{
		ToolName: "task32_hard_error",
		Input:    ToolInput{ArgsJSON: []byte(`{"n":1}`)},
		Env:      NewRunEnv(sess),
	})

	// Assert.
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	assert.Equal(t, OutcomeBusinessError, outcome.Status)
	assert.Equal(t, ToolEnvelopeKindError, outcome.Envelope.Kind)
	require.NotNil(t, outcome.Envelope.Error)
	assert.Equal(t, CodeValidationFailed, outcome.Envelope.Error.Code)
}

func mustTask32Snapshot(t *testing.T, binding SessionBinding, payload string) SessionSnapshot {
	t.Helper()
	raw, err := json.Marshal(sessionSnapshotWire{
		Version: sessionSnapshotVersion,
		Payload: json.RawMessage(payload),
		Binding: binding,
	})
	require.NoError(t, err)
	snap, err := NewSessionSnapshotFromJSON(raw)
	require.NoError(t, err)
	return snap
}
