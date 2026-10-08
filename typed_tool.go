package toolsy

import (
	"context"
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

// ResultValidator validates typed results after the handler and before marshaling.
// Failures become noncorrectable ResultContractError values, preserving the cause.
type ResultValidator[R any] func(R) error

// EffectValidator validates host-owned effects after the handler.
// Failures never authorize argument repair or rollback of external effects.
type EffectValidator[E any] func([]E) error

// PostconditionValidator validates the complete typed result/effects/control envelope
// after the handler; failures never authorize argument repair or redispatch.
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
// Empty and Noop are exclusive statuses without wire bytes; they retain Value.
// Empty may declare effects; Noop may not. Both may declare controls. Nonempty Raw replaces
// wire encoding only, preserving Value, and cannot accompany Empty or Noop.
// RawMimeType requires nonempty Raw; otherwise ordinary Value is encoded as JSON.
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

// NewEmptyToolResult returns a successful result without a wire payload.
// It may report effects and controls; this does not assert absence of side effects.
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

// NewNoopToolResult declares no effects and carries no wire payload.
// Controls and delivery metadata are allowed; this declaration does not prove
// that an arbitrary host handler performed no external side effect.
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
		return nil, ErrToolHandlerNil
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
	if len(cfg.Manifest.OutputSchema) == 0 && !hasCustomResultEncoding[TResult]() {
		outSchema, genErr := generateOutputSchema[TResult](cfg.Schema)
		if genErr != nil {
			return nil, genErr
		}
		cfg.Manifest.OutputSchema = outSchema
	}
	manifest := buildToolManifest(spec.Name, spec.Description, ext.Schema(), cfg.Manifest)
	outputValidator, err := compileResultContract(&manifest)
	if err != nil {
		return nil, err
	}

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
		return executePreparedResult(
			ctx,
			env,
			manifest,
			input,
			bound.Value,
			outputValidator,
			func(_ ToolInput, out func(Chunk) error) error {
				res, handlerErr := spec.Handler(ctx, callCtx, env, cloneValidatedArgs(bound))
				if handlerErr != nil {
					return wrapHandlerError(handlerErr)
				}
				return emitTypedToolResult(res, spec.ResultValidator, spec.EffectValidator, spec.Postcondition, out)
			},
			yield,
		)
	}
	return &tool{manifest: manifest, execute: execute}, nil
}

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
	if out, ok := cloneMutableValue(in).(T); ok {
		return out
	}
	return in
}

func emitTypedToolResult[TResult, TEffect any](
	res ToolResult[TResult, TEffect],
	resultValidator ResultValidator[TResult],
	effectValidator EffectValidator[TEffect],
	postcondition PostconditionValidator[TResult, TEffect],
	yield func(Chunk) error,
) error {
	if err := validateResultFlags(res.Empty, res.Noop, len(res.Raw), len(res.Effects)); err != nil {
		return err
	}
	if res.RawMimeType != "" && len(res.Raw) == 0 {
		return invalidResultAlgebra("RawMimeType requires nonempty Raw")
	}
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
	var chunk Chunk
	chunk.Event = EventResult
	chunk.TypedResult = res.Value
	chunk.EmptyResult = res.Empty
	chunk.Noop = res.Noop
	chunk.Effects = effectsToAny(res.Effects)
	chunk.Controls = append([]ControlSignal(nil), res.Controls...)
	switch {
	case res.Empty || res.Noop:
		// Retain the BYOT value without serializing it into wire bytes.
	case len(res.Raw) > 0:
		chunk.Data = append([]byte(nil), res.Raw...)
		chunk.MimeType = res.RawMimeType
		if chunk.MimeType == "" {
			chunk.MimeType = MimeTypeOctetStream
		}
	default:
		data, err := marshalToolResult(res.Value)
		if err != nil {
			return Chunk{}, NewInternalError(fmt.Errorf("toolsy: marshal typed result: %w", err))
		}
		chunk.Data = data
		chunk.MimeType = MimeTypeJSON
	}
	chunk.Envelope = NewResultEnvelope(
		chunk.TypedResult,
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
	return wrapPostHandlerContractError("result_validator", err)
}

func wrapEffectValidatorError(err error) error {
	return wrapPostHandlerContractError("effect_validator", err)
}

func wrapPostconditionError(err error) error {
	return wrapPostHandlerContractError("postcondition", err)
}

func wrapPostHandlerContractError(phase string, err error) error {
	if err == nil {
		return nil
	}
	return NewInternalError(&ResultContractError{Kind: phase, Cause: err})
}
