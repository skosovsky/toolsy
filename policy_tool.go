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
	if spec.Tool == nil {
		return nil, errors.New("toolsy: policy tool requires base tool")
	}
	if spec.ArgsBinder == nil {
		return nil, errors.New("toolsy: policy tool requires args binder")
	}
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
			spec.Policy,
			spec.ArgValidator,
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
		return spec.Tool.Execute(ctx, env, forward, func(c Chunk) error {
			return yield(applyPolicyToolEnvelope(c, spec.DeliveryClass, spec.Audience, spec.EnvelopeMetadata))
		})
	}
	return &tool{manifest: manifest, execute: execute}, nil
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
		envelope.Audience = audience
	}
	if len(metadata) > 0 {
		merged := deepCloneMap(envelope.Metadata)
		if merged == nil {
			merged = make(map[string]any, len(metadata))
		}
		maps.Copy(merged, deepCloneMap(metadata))
		envelope.Metadata = merged
	}
	c.Envelope = &envelope
	return c
}
