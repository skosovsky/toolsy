package toolsy

import (
	"context"
	"errors"
	"maps"
)

// ToolPolicySpec adds binder/policy/requirements to an existing generic tool.
type ToolPolicySpec[TSubject, TScope, TArgs any] struct {
	Tool             Tool
	Requirements     ToolRequirements
	ArgsBinder       ArgsBinder[TArgs]
	ArgValidator     ArgValidator[TArgs]
	Policy           TypedPolicy[TSubject, TScope, TArgs]
	CallContext      func(context.Context, *RunEnv, ToolInput) (CallContext, error)
	DeliveryClass    ToolDeliveryClass
	Audience         ToolAudience
	EnvelopeMetadata map[string]any
}

// ToolPolicyConstructorSpec builds a policy-aware generic tool in one step.
type ToolPolicyConstructorSpec[TSubject, TScope, TArgs any] struct {
	Name             string
	Description      string
	RawJSONSchema    []byte
	Handler          func(ctx context.Context, env *RunEnv, rawArgs []byte, yield func(Chunk) error) error
	Requirements     ToolRequirements
	ArgsBinder       ArgsBinder[TArgs]
	ArgValidator     ArgValidator[TArgs]
	Policy           TypedPolicy[TSubject, TScope, TArgs]
	CallContext      func(context.Context, *RunEnv, ToolInput) (CallContext, error)
	DeliveryClass    ToolDeliveryClass
	Audience         ToolAudience
	EnvelopeMetadata map[string]any
	Options          []ToolOption
}

// NewPolicyToolFromSpec builds a policy-aware generic tool without first exposing a bare tool.
func NewPolicyToolFromSpec[TSubject, TScope, TArgs any](
	spec ToolPolicyConstructorSpec[TSubject, TScope, TArgs],
) (Tool, error) {
	base, err := NewProxyTool(spec.Name, spec.Description, spec.RawJSONSchema, spec.Handler, spec.Options...)
	if err != nil {
		return nil, err
	}
	return NewPolicyTool(ToolPolicySpec[TSubject, TScope, TArgs]{
		Tool:             base,
		Requirements:     spec.Requirements,
		ArgsBinder:       spec.ArgsBinder,
		ArgValidator:     spec.ArgValidator,
		Policy:           spec.Policy,
		CallContext:      spec.CallContext,
		DeliveryClass:    spec.DeliveryClass,
		Audience:         spec.Audience,
		EnvelopeMetadata: spec.EnvelopeMetadata,
	})
}

// NewPolicyTool builds a production-ready generic tool without host-local policy wrappers.
func NewPolicyTool[TSubject, TScope, TArgs any](spec ToolPolicySpec[TSubject, TScope, TArgs]) (Tool, error) {
	if err := validatePolicyToolSpec(spec); err != nil {
		return nil, err
	}
	return buildPolicyTool(spec)
}

func validatePolicyToolSpec[TSubject, TScope, TArgs any](spec ToolPolicySpec[TSubject, TScope, TArgs]) error {
	if spec.Tool == nil {
		return errors.New("toolsy: policy tool requires base tool")
	}
	if spec.ArgsBinder == nil {
		return errors.New("toolsy: policy tool requires args binder")
	}
	if !supportsPreparedExecution(spec.Tool) {
		return errors.New("toolsy: policy tool requires a prepared execution boundary")
	}
	if _, reserved := spec.EnvelopeMetadata[ReplaySourceMetadata]; reserved {
		return errors.New("toolsy: policy tool cannot configure reserved replay metadata")
	}
	return nil
}

func buildPolicyTool[TSubject, TScope, TArgs any](spec ToolPolicySpec[TSubject, TScope, TArgs]) (Tool, error) {
	spec.EnvelopeMetadata = deepCloneMap(spec.EnvelopeMetadata)
	manifest := spec.Tool.Manifest()
	if hasRequirements(spec.Requirements) {
		manifest.Requirements = cloneRequirements(spec.Requirements)
	}
	cfg := ensureSchemaConfig(SchemaConfig{Strict: false, Registry: nil})
	ext, err := NewExtractorWithConfig[TArgs](cfg)
	if err != nil {
		return nil, err
	}
	execute := func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		if env == nil {
			env = NewRunEnv(nil)
		}
		if spec.CallContext != nil {
			callCtx, callErr := spec.CallContext(ctx, env, input.Clone())
			if callErr != nil {
				return wrapArgValidatorError(callErr)
			}
			env = env.cloneForExecute(input.Attachments, env.async, callCtx)
		}
		bound, callCtx, prepErr := prepareTypedToolCall[TSubject, TScope, TArgs](
			ctx,
			env,
			input,
			manifest,
			ext,
			spec.ArgsBinder,
			nil,
			nil,
		)
		if prepErr != nil {
			return prepErr
		}
		forward := input.Clone()
		if len(bound.Raw) == 0 {
			return NewValidationError("args binder must return canonical raw args", "args")
		}
		forward.ArgsJSON = append([]byte(nil), bound.Raw...)
		env = env.cloneForExecute(input.Attachments, env.async, CallContext{
			Subject:  callCtx.Subject,
			Scope:    callCtx.Scope,
			Metadata: cloneCallMetadata(callCtx.Metadata),
			Values:   maps.Clone(callCtx.Values),
		})
		// Delegate to the innermost preparation boundary: every binder and policy
		// must run before a profile can claim, dispatch or replay an outcome.
		innerEnv := *env
		innerEnv.preparedChecks = append(innerEnv.preparedChecks, policyFinalCheck(spec, ext, bound.Metadata))
		if innerEnv.executionManifest == nil {
			innerEnv.executionManifest = &manifest
		}
		transform := func(c Chunk) Chunk {
			return applyPolicyToolEnvelope(c, spec.DeliveryClass, spec.Audience, spec.EnvelopeMetadata)
		}
		if innerEnv.executionProfile != nil {
			innerEnv.executionProfile = policyEnvelopeProfile{next: innerEnv.executionProfile, transform: transform}
		}
		return spec.Tool.Execute(ctx, &innerEnv, forward, func(c Chunk) error { return yield(transform(c)) })
	}
	return &tool{manifest: manifest, execute: execute}, nil
}

func policyFinalCheck[TSubject, TScope, TArgs any](
	spec ToolPolicySpec[TSubject, TScope, TArgs], ext *Extractor[TArgs], metadata map[string]any,
) func(context.Context, PreparedCall, any) error {
	return func(ctx context.Context, call PreparedCall, finalArgs any) error {
		value, ok := finalArgs.(TArgs)
		if !ok {
			var err error
			value, err = ext.ParseAndValidate(call.Input.ArgsJSON)
			if err != nil {
				return err
			}
		}
		if spec.ArgValidator != nil {
			if err := spec.ArgValidator(cloneTypedArgValue(value)); err != nil {
				return wrapArgValidatorError(err)
			}
		}
		if spec.Policy == nil {
			return nil
		}
		finalContext, err := TypedContext[TSubject, TScope](call.Context)
		if err != nil {
			return err
		}
		return decisionError(spec.Policy(ctx, TypedPolicyRequest[TSubject, TScope, TArgs]{
			Manifest: call.Manifest,
			Input:    call.Input,
			Context:  finalContext,
			Args:     value,
			BoundArgs: ValidatedArgs[TArgs]{
				Value:    cloneTypedArgValue(value),
				Raw:      append([]byte(nil), call.Input.ArgsJSON...),
				Metadata: cloneArgsMetadata(metadata),
			},
		}))
	}
}

func underlyingExecutionProfile(profile ExecutionProfile) ExecutionProfile {
	for {
		decorator, ok := profile.(policyEnvelopeProfile)
		if !ok {
			return profile
		}
		profile = decorator.next
	}
}

// policyEnvelopeProfile preserves current delivery restrictions both when
// persisting fresh outcomes and when returning previously stored outcomes.
type policyEnvelopeProfile struct {
	next      ExecutionProfile
	transform func(Chunk) Chunk
}

func (p policyEnvelopeProfile) ExecutePrepared(
	ctx context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error,
) error {
	return p.next.ExecutePrepared(ctx, call, func(out func(Chunk) error) error {
		return invoke(func(c Chunk) error { return out(p.transform(c)) })
	}, func(c Chunk) error { return yield(p.transform(c)) })
}

func applyPolicyToolEnvelope(
	c Chunk,
	deliveryClass ToolDeliveryClass,
	audience ToolAudience,
	metadata map[string]any,
) Chunk {
	if c.Event != EventResult || (deliveryClass == "" && audience == "" && len(metadata) == 0) {
		return c
	}
	envelope := c.ToolEnvelope()
	if deliveryClass != "" {
		envelope.DeliveryClass = deliveryClass
	}
	if audience != "" {
		if source, _ := envelope.Metadata[ReplaySourceMetadata].(string); source != "" &&
			envelope.Audience != audience {
			// Model and user are disjoint delivery targets; internal is the
			// conservative intersection. A stored private outcome never becomes public.
			envelope.Audience = AudienceInternal
		} else {
			envelope.Audience = audience
		}
	}
	if len(metadata) > 0 {
		merged := deepCloneMap(envelope.Metadata)
		if merged == nil {
			merged = make(map[string]any, len(metadata))
		}
		overlay := deepCloneMap(metadata)
		// Replay provenance belongs to the execution profile, never to an overlay.
		delete(overlay, ReplaySourceMetadata)
		maps.Copy(merged, overlay)
		envelope.Metadata = merged
	}
	c.Envelope = &envelope
	return c
}
