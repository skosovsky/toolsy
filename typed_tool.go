package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
)

// ArgsBindRequest is the raw execution boundary passed to an [ArgsBinder].
type ArgsBindRequest struct {
	Manifest    ToolManifest
	Input       ToolInput
	CallContext CallContext
}

// ValidatedArgs is the canonical output of an [ArgsBinder].
type ValidatedArgs[T any] struct {
	Value    T
	Raw      []byte
	Metadata map[string]any
}

// ArgsBinder validates raw input and returns canonical typed args for the handler.
type ArgsBinder[T any] func(ctx context.Context, req ArgsBindRequest) (ValidatedArgs[T], error)

// ArgValidator validates typed arguments after schema parse.
type ArgValidator[T any] func(T) error

// ResultValidator validates typed results before marshaling.
type ResultValidator[R any] func(R) error

// EffectValidator validates host-owned effects before they reach the outcome contract.
type EffectValidator[E any] func([]E) error

// PostconditionValidator validates the complete typed result/effects/control envelope.
type PostconditionValidator[R, E any] func(ToolResult[R, E]) error

// TypedPolicyRequest is the compile-time policy request for typed tools.
type TypedPolicyRequest[TSubject, TScope, TArgs any] struct {
	Manifest  ToolManifest
	Input     ToolInput
	Context   TypedCallContext[TSubject, TScope]
	Args      TArgs
	BoundArgs ValidatedArgs[TArgs]
}

// TypedPolicy validates typed subject/scope/args before handler execution.
type TypedPolicy[TSubject, TScope, TArgs any] func(context.Context, TypedPolicyRequest[TSubject, TScope, TArgs]) Decision

// ToolResult is the typed result/effects contract returned by production typed tools.
type ToolResult[TResult, TEffect any] struct {
	Value            TResult
	Empty            bool
	Noop             bool
	Effects          []TEffect
	Controls         []ControlSignal
	Raw              []byte
	RawMimeType      string
	DeliveryClass    ToolDeliveryClass
	Audience         ToolAudience
	EnvelopeMetadata map[string]any
}

// NewToolResult returns a successful typed result with no effects.
func NewToolResult[TResult, TEffect any](value TResult) ToolResult[TResult, TEffect] {
	return ToolResult[TResult, TEffect]{
		Value:            value,
		Empty:            false,
		Noop:             false,
		Effects:          nil,
		Controls:         nil,
		Raw:              nil,
		RawMimeType:      "",
		DeliveryClass:    "",
		Audience:         "",
		EnvelopeMetadata: nil,
	}
}

// NewEmptyToolResult returns an intentional successful no-op/empty result.
func NewEmptyToolResult[TResult, TEffect any]() ToolResult[TResult, TEffect] {
	var zero TResult
	return ToolResult[TResult, TEffect]{
		Value:            zero,
		Empty:            true,
		Noop:             false,
		Effects:          nil,
		Controls:         nil,
		Raw:              nil,
		RawMimeType:      "",
		DeliveryClass:    "",
		Audience:         "",
		EnvelopeMetadata: nil,
	}
}

// NewNoopToolResult returns an intentional successful no-op result.
func NewNoopToolResult[TResult, TEffect any]() ToolResult[TResult, TEffect] {
	var zero TResult
	return ToolResult[TResult, TEffect]{
		Value:            zero,
		Empty:            false,
		Noop:             true,
		Effects:          nil,
		Controls:         nil,
		Raw:              nil,
		RawMimeType:      "",
		DeliveryClass:    "",
		Audience:         "",
		EnvelopeMetadata: nil,
	}
}

// TypedToolSpec describes a first-class typed tool contract.
type TypedToolSpec[TSubject, TScope, TArgs, TResult, TEffect any] struct {
	Name, Description string
	ArgsBinder        ArgsBinder[TArgs]
	ArgValidator      ArgValidator[TArgs]
	ResultValidator   ResultValidator[TResult]
	EffectValidator   EffectValidator[TEffect]
	Postcondition     PostconditionValidator[TResult, TEffect]
	Policy            TypedPolicy[TSubject, TScope, TArgs]
	Handler           func(ctx context.Context, call TypedCallContext[TSubject, TScope], env *RunEnv, args ValidatedArgs[TArgs]) (ToolResult[TResult, TEffect], error)
	Options           []ToolOption
}

// NewTypedTool builds a Tool from TypedToolSpec with native raw validation, typed decode,
// policy binding, result validation, effect validation, and stable error mapping.
func NewTypedTool[TSubject, TScope, TArgs, TResult, TEffect any](
	spec TypedToolSpec[TSubject, TScope, TArgs, TResult, TEffect],
) (Tool, error) {
	if spec.Handler == nil {
		return nil, errTypedToolNilHandler
	}
	var cfg ToolConfig
	for _, opt := range spec.Options {
		opt(&cfg)
	}
	cfg.Schema = ensureSchemaConfig(cfg.Schema)
	ext, err := NewExtractorWithConfig[TArgs](cfg.Schema)
	if err != nil {
		return nil, err
	}
	if len(cfg.Manifest.OutputSchema) == 0 {
		outSchema, genErr := generateOutputSchema[TResult](cfg.Schema)
		if genErr != nil {
			return nil, genErr
		}
		cfg.Manifest.OutputSchema = outSchema
	}
	manifest := buildToolManifest(spec.Name, spec.Description, ext.Schema(), cfg.Manifest)

	execute := func(ctx context.Context, env *RunEnv, input ToolInput, yield func(Chunk) error) error {
		bound, callCtx, err := prepareTypedToolCall[TSubject, TScope, TArgs](
			ctx,
			env,
			input,
			manifest,
			ext,
			spec.ArgsBinder,
			spec.Policy,
			spec.ArgValidator,
		)
		if err != nil {
			return err
		}
		res, err := spec.Handler(ctx, callCtx, env, cloneValidatedArgs(bound))
		if err != nil {
			return wrapHandlerError(err)
		}
		return emitTypedToolResult(res, spec.ResultValidator, spec.EffectValidator, spec.Postcondition, yield)
	}
	return &tool{manifest: manifest, execute: execute}, nil
}

var errTypedToolNilHandler = errors.New("toolsy: typed tool handler must not be nil")

func prepareTypedToolCall[TSubject, TScope, TArgs any](
	ctx context.Context,
	env *RunEnv,
	input ToolInput,
	manifest ToolManifest,
	ext *Extractor[TArgs],
	argsBinder ArgsBinder[TArgs],
	policy TypedPolicy[TSubject, TScope, TArgs],
	argValidator ArgValidator[TArgs],
) (ValidatedArgs[TArgs], TypedCallContext[TSubject, TScope], error) {
	var zeroArgs ValidatedArgs[TArgs]
	var zeroCtx TypedCallContext[TSubject, TScope]
	callCtx, err := TypedContext[TSubject, TScope](env.CallContext())
	if err != nil {
		return zeroArgs, zeroCtx, err
	}
	bound, err := bindTypedArgs(ctx, input, manifest, callCtx, ext, argsBinder)
	if err != nil {
		return zeroArgs, zeroCtx, err
	}
	if argValidator != nil {
		argErr := argValidator(bound.Value)
		if argErr != nil {
			return zeroArgs, zeroCtx, wrapArgValidatorError(argErr)
		}
	}
	if policy != nil {
		req := TypedPolicyRequest[TSubject, TScope, TArgs]{
			Manifest:  cloneManifestForPolicy(manifest),
			Input:     input.Clone(),
			Context:   callCtx,
			Args:      cloneTypedArgValue(bound.Value),
			BoundArgs: cloneValidatedArgs(bound),
		}
		policyErr := decisionError(policy(ctx, req))
		if policyErr != nil {
			return zeroArgs, zeroCtx, policyErr
		}
	}
	return cloneValidatedArgs(bound), callCtx, nil
}

func bindTypedArgs[TSubject, TScope, TArgs any](
	ctx context.Context,
	input ToolInput,
	manifest ToolManifest,
	callCtx TypedCallContext[TSubject, TScope],
	ext *Extractor[TArgs],
	argsBinder ArgsBinder[TArgs],
) (ValidatedArgs[TArgs], error) {
	if argsBinder != nil {
		bound, err := argsBinder(ctx, ArgsBindRequest{
			Manifest: cloneManifestForPolicy(manifest),
			Input:    input.Clone(),
			CallContext: CallContext{
				Subject:  callCtx.Subject,
				Scope:    callCtx.Scope,
				Metadata: cloneCallMetadata(callCtx.Metadata),
				Values:   maps.Clone(callCtx.Values),
			},
		})
		if err != nil {
			return ValidatedArgs[TArgs]{}, wrapArgValidatorError(err)
		}
		bound.Raw = append([]byte(nil), bound.Raw...)
		bound.Metadata = cloneArgsMetadata(bound.Metadata)
		return bound, nil
	}
	args, err := ext.ParseAndValidate(input.ArgsJSON)
	if err != nil {
		return ValidatedArgs[TArgs]{}, err
	}
	return ValidatedArgs[TArgs]{
		Value:    args,
		Raw:      append([]byte(nil), input.ArgsJSON...),
		Metadata: nil,
	}, nil
}

func cloneValidatedArgs[T any](in ValidatedArgs[T]) ValidatedArgs[T] {
	return ValidatedArgs[T]{
		Value:    cloneTypedArgValue(in.Value),
		Raw:      append([]byte(nil), in.Raw...),
		Metadata: cloneArgsMetadata(in.Metadata),
	}
}

func cloneArgsMetadata(in map[string]any) map[string]any {
	return deepCloneMap(in)
}

func cloneTypedArgValue[T any](in T) T {
	payload, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out T
	if err := json.Unmarshal(payload, &out); err != nil {
		return in
	}
	return out
}

func emitTypedToolResult[TResult, TEffect any](
	res ToolResult[TResult, TEffect],
	resultValidator ResultValidator[TResult],
	effectValidator EffectValidator[TEffect],
	postcondition PostconditionValidator[TResult, TEffect],
	yield func(Chunk) error,
) error {
	if resultValidator != nil && !res.Empty && !res.Noop {
		resultErr := resultValidator(res.Value)
		if resultErr != nil {
			return wrapResultValidatorError(resultErr)
		}
	}
	if effectValidator != nil {
		effectErr := effectValidator(res.Effects)
		if effectErr != nil {
			return wrapEffectValidatorError(effectErr)
		}
	}
	if postcondition != nil {
		postErr := postcondition(res)
		if postErr != nil {
			return wrapPostconditionError(postErr)
		}
	}
	chunk, err := chunkFromToolResult(res)
	if err != nil {
		return err
	}
	prepared, err := prepareChunk(chunk)
	if err != nil {
		return err
	}
	if err := yield(prepared); err != nil {
		return wrapYieldError(err)
	}
	return nil
}

func chunkFromToolResult[TResult, TEffect any](res ToolResult[TResult, TEffect]) (Chunk, error) {
	chunk := Chunk{
		Event:       EventResult,
		TypedResult: res.Value,
		EmptyResult: res.Empty,
		Noop:        res.Noop,
		Effects:     effectsToAny(res.Effects),
		Controls:    append([]ControlSignal(nil), res.Controls...),
	}
	switch {
	case len(res.Raw) > 0:
		chunk.Data = append([]byte(nil), res.Raw...)
		chunk.MimeType = res.RawMimeType
		if chunk.MimeType == "" {
			chunk.MimeType = MimeTypeOctetStream
		}
	case res.Empty:
		return chunk, nil
	default:
		data, err := marshalToolResult(res.Value)
		if err != nil {
			return Chunk{}, NewInternalError(fmt.Errorf("toolsy: marshal typed result: %w", err))
		}
		chunk.Data = data
		chunk.MimeType = MimeTypeJSON
	}
	chunk.Envelope = NewResultEnvelope(
		res.Value,
		chunk.Data,
		chunk.MimeType,
		res.DeliveryClass,
		res.Audience,
		res.EnvelopeMetadata,
	)
	return chunk, nil
}

func effectsToAny[TEffect any](effects []TEffect) []any {
	if len(effects) == 0 {
		return nil
	}
	out := make([]any, 0, len(effects))
	for _, effect := range effects {
		out = append(out, effect)
	}
	return out
}

func wrapArgValidatorError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError(err.Error())
}

func wrapResultValidatorError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError("result validation failed: " + err.Error())
}

func wrapEffectValidatorError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError("effect validation failed: " + err.Error())
}

func wrapPostconditionError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsToolError(err); ok {
		return err
	}
	return NewValidationError("postcondition failed: " + err.Error())
}
