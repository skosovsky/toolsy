package toolsy

import (
	"context"
	"errors"
	"fmt"
)

// RawArgValidator validates raw JSON arguments before typed parsing.
type RawArgValidator func(ctx context.Context, toolName string, argsJSON []byte) error

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
	Manifest ToolManifest
	Input    ToolInput
	Context  TypedCallContext[TSubject, TScope]
	Args     TArgs
}

// TypedPolicy validates typed subject/scope/args before handler execution.
type TypedPolicy[TSubject, TScope, TArgs any] func(context.Context, TypedPolicyRequest[TSubject, TScope, TArgs]) Decision

// ToolResult is the typed result/effects contract returned by production typed tools.
type ToolResult[TResult, TEffect any] struct {
	Value       TResult
	Empty       bool
	Noop        bool
	Effects     []TEffect
	Controls    []ControlSignal
	Raw         []byte
	RawMimeType string
}

// NewToolResult returns a successful typed result with no effects.
func NewToolResult[TResult, TEffect any](value TResult) ToolResult[TResult, TEffect] {
	return ToolResult[TResult, TEffect]{
		Value:       value,
		Empty:       false,
		Noop:        false,
		Effects:     nil,
		Controls:    nil,
		Raw:         nil,
		RawMimeType: "",
	}
}

// NewEmptyToolResult returns an intentional successful no-op/empty result.
func NewEmptyToolResult[TResult, TEffect any]() ToolResult[TResult, TEffect] {
	var zero TResult
	return ToolResult[TResult, TEffect]{
		Value:       zero,
		Empty:       true,
		Noop:        false,
		Effects:     nil,
		Controls:    nil,
		Raw:         nil,
		RawMimeType: "",
	}
}

// NewNoopToolResult returns an intentional successful no-op result.
func NewNoopToolResult[TResult, TEffect any]() ToolResult[TResult, TEffect] {
	var zero TResult
	return ToolResult[TResult, TEffect]{
		Value:       zero,
		Empty:       false,
		Noop:        true,
		Effects:     nil,
		Controls:    nil,
		Raw:         nil,
		RawMimeType: "",
	}
}

// TypedToolSpec describes a first-class typed tool contract.
type TypedToolSpec[TSubject, TScope, TArgs, TResult, TEffect any] struct {
	Name, Description string
	RawValidator      RawArgValidator
	ArgValidator      ArgValidator[TArgs]
	ResultValidator   ResultValidator[TResult]
	EffectValidator   EffectValidator[TEffect]
	Postcondition     PostconditionValidator[TResult, TEffect]
	Policy            TypedPolicy[TSubject, TScope, TArgs]
	Handler           func(ctx context.Context, call TypedCallContext[TSubject, TScope], env *RunEnv, args TArgs) (ToolResult[TResult, TEffect], error)
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
		args, callCtx, err := prepareTypedToolCall[TSubject, TScope, TArgs](
			ctx,
			env,
			input,
			manifest,
			ext,
			spec.RawValidator,
			spec.Policy,
			spec.ArgValidator,
		)
		if err != nil {
			return err
		}
		res, err := spec.Handler(ctx, callCtx, env, args)
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
	rawValidator RawArgValidator,
	policy TypedPolicy[TSubject, TScope, TArgs],
	argValidator ArgValidator[TArgs],
) (TArgs, TypedCallContext[TSubject, TScope], error) {
	var zeroArgs TArgs
	var zeroCtx TypedCallContext[TSubject, TScope]
	if rawValidator != nil {
		rawErr := rawValidator(ctx, manifest.Name, append([]byte(nil), input.ArgsJSON...))
		if rawErr != nil {
			return zeroArgs, zeroCtx, wrapArgValidatorError(rawErr)
		}
	}
	policyArgs, err := ext.ParseAndValidate(input.ArgsJSON)
	if err != nil {
		return zeroArgs, zeroCtx, err
	}
	callCtx, err := TypedContext[TSubject, TScope](env.CallContext())
	if err != nil {
		return zeroArgs, zeroCtx, err
	}
	if policy != nil {
		req := TypedPolicyRequest[TSubject, TScope, TArgs]{
			Manifest: cloneManifestForPolicy(manifest),
			Input:    input.Clone(),
			Context:  callCtx,
			Args:     policyArgs,
		}
		policyErr := decisionError(policy(ctx, req))
		if policyErr != nil {
			return zeroArgs, zeroCtx, policyErr
		}
	}
	args, err := ext.ParseAndValidate(input.ArgsJSON)
	if err != nil {
		return zeroArgs, zeroCtx, err
	}
	if argValidator != nil {
		argErr := argValidator(args)
		if argErr != nil {
			return zeroArgs, zeroCtx, wrapArgValidatorError(argErr)
		}
	}
	return args, callCtx, nil
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
