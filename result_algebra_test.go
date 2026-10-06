package toolsy

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func payloadFreeTool(t *testing.T, mode string, metadata map[string]any, calls *int) Tool {
	t.Helper()
	tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, map[string]any, string]{
		Name:        "payload-free",
		Description: "payload-free",
		Options:     []ToolOption{WithIdempotent()},
		Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[map[string]any, string], error) {
			*calls++
			result := NewToolResult[map[string]any, string](map[string]any{"details": "retained"})
			result.Empty = mode == "empty"
			result.Noop = mode == "noop"
			result.Audience = AudienceInternal
			result.DeliveryClass = DeliveryClassText
			result.EnvelopeMetadata = metadata
			result.Controls = []ControlSignal{&PauseSignal{Reason: "review"}}
			if mode == "empty" {
				result.Effects = []string{"recorded"}
			}
			return result, nil
		},
	})
	require.NoError(t, err)
	return tool
}

func runPayloadFreePath(t *testing.T, path string, tool Tool) Chunk {
	t.Helper()
	var options []RegistryOption
	if path == "cache" {
		cache, err := NewResultCache(
			NewMemoryResultCacheStore(),
			constantPartition,
			JSONResultCodec[map[string]any, string]{},
			0,
		)
		require.NoError(t, err)
		options = append(options, WithExecutionProfile(cache))
	}
	registry, err := NewRegistryBuilder(options...).Add(tool).Build()
	require.NoError(t, err)
	call := ToolCall{ToolName: "payload-free", Input: ToolInput{ArgsJSON: []byte(`{}`)}}
	var delivered Chunk
	yield := func(chunk Chunk) error { delivered = chunk; return nil }
	switch path {
	case "direct":
		err = tool.Execute(t.Context(), nil, call.Input, yield)
	case "session":
		session, buildErr := NewSession(registry)
		require.NoError(t, buildErr)
		outcome, callErr := session.RunCall(t.Context(), call)
		err = callErr
		require.Equal(t, map[string]any{"details": "retained"}, outcome.TypedResult)
		require.Empty(t, outcome.Result)
		_, decodeErr := DecodeOutcomeAs[map[string]any](outcome)
		require.NoError(t, decodeErr)
		delivered = Chunk{
			Event:       EventResult,
			EmptyResult: outcome.EmptyResult,
			Noop:        outcome.Noop,
			Effects:     outcome.Effects,
			TypedResult: outcome.TypedResult,
			Controls:    outcome.Controls,
			Envelope:    &outcome.Envelope,
		}
	default:
		err = registry.Execute(t.Context(), call, yield)
	}
	require.NoError(t, err)
	if path == "cache" {
		require.NoError(t, registry.Execute(t.Context(), call, yield))
	}
	return delivered
}

func TestPayloadFreeResultPreservesEnvelope(t *testing.T) {
	for _, mode := range []string{"empty", "noop"} {
		for _, path := range []string{"direct", "registry", "session", "cache"} {
			t.Run(mode+"/"+path, func(t *testing.T) {
				// Arrange: either status omits wire bytes but retains typed result data.
				var calls int
				metadata := map[string]any{"private": true}
				tool := payloadFreeTool(t, mode, metadata, &calls)
				// Act.
				delivered := runPayloadFreePath(t, path, tool)
				metadata["private"] = false
				// Assert.
				require.Equal(t, 1, calls)
				require.Empty(t, delivered.Data)
				require.Empty(t, delivered.MimeType)
				require.Equal(t, map[string]any{"details": "retained"}, delivered.TypedResult)
				require.Equal(t, delivered.TypedResult, delivered.Envelope.Result)
				require.Equal(t, mode == "empty", delivered.EmptyResult)
				require.Equal(t, mode == "noop", delivered.Noop)
				require.Equal(t, AudienceInternal, delivered.Envelope.Audience)
				require.Equal(t, DeliveryClassText, delivered.Envelope.DeliveryClass)
				require.Equal(t, true, delivered.Envelope.Metadata["private"])
				require.Len(t, delivered.Controls, 1)
				if mode == "empty" {
					require.Equal(t, []any{"recorded"}, delivered.Effects)
				} else {
					require.Empty(t, delivered.Effects)
				}
				if path == "cache" {
					require.Equal(t, true, delivered.Envelope.Metadata[CacheReplayMetadata])
				}
			})
		}
	}
}

func TestContradictoryTypedResultDeclarationsFailAfterHandler(t *testing.T) {
	for _, mode := range []string{"empty+raw", "noop+raw", "empty+noop", "noop+effects", "mime-without-raw"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			var calls, delivered int
			tool, err := NewTypedTool(
				TypedToolSpec[NoSubject, NoScope, struct{}, int, string]{
					Name:        "invalid",
					Description: "invalid",
					Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[int, string], error) {
						calls++
						result := NewToolResult[int, string](7)
						switch mode {
						case "empty+raw":
							result.Empty = true
							result.Raw = []byte(`1`)
						case "noop+raw":
							result.Noop = true
							result.Raw = []byte(`1`)
						case "empty+noop":
							result.Empty = true
							result.Noop = true
						case "noop+effects":
							result.Noop = true
							result.Effects = []string{"recorded"}
						case "mime-without-raw":
							result.RawMimeType = MimeTypeText
						}
						return result, nil
					},
				},
			)
			require.NoError(t, err)
			// Act.
			err = tool.Execute(
				t.Context(),
				nil,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(Chunk) error { delivered++; return nil },
			)
			// Assert.
			require.Equal(t, 1, calls)
			require.Zero(t, delivered)
			requireToolErrorCode(t, err, CodeInternal)
			var contract *ResultContractError
			require.ErrorAs(t, err, &contract)
			require.Equal(t, "result_algebra", contract.Kind)
			require.Error(t, contract.Cause)
			require.False(t, ClientCorrectable(CodeInternal))
		})
	}
}

type rawOutputFixture struct {
	Body json.RawMessage `json:"body"`
}

func TestRawMessageOutputSupportsAllJSONShapes(t *testing.T) {
	for _, raw := range []string{`{"x":1}`, `[1,true]`, `"value"`, `9007199254740993`, `true`, `null`} {
		t.Run(raw, func(t *testing.T) {
			// Arrange.
			tool, err := NewTool(
				"raw-field",
				"raw-field",
				func(context.Context, *RunEnv, struct{}) (rawOutputFixture, error) {
					return rawOutputFixture{Body: json.RawMessage(raw)}, nil
				},
			)
			require.NoError(t, err)
			var output Chunk
			// Act.
			err = tool.Execute(
				t.Context(),
				nil,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(chunk Chunk) error { output = chunk; return nil },
			)
			// Assert.
			require.NoError(t, err)
			require.JSONEq(t, `{"body":`+raw+`}`, string(output.Data))
			body := tool.Manifest().OutputSchema["properties"].(map[string]any)["body"]
			require.Equal(t, true, body)
		})
	}
}

func TestRawMessageExplicitMappingAndOutputSchemaPrecedence(t *testing.T) {
	// Arrange.
	registry := NewSchemaRegistry()
	registry.RegisterType(json.RawMessage(nil), "string", "")
	handler := func(context.Context, *RunEnv, struct{}) (rawOutputFixture, error) {
		return rawOutputFixture{Body: json.RawMessage(`7`)}, nil
	}
	tool, err := NewTool(
		"explicit",
		"explicit",
		handler,
		WithSchemaRegistry(registry),
		WithOutputSchema(
			map[string]any{"type": "object", "properties": map[string]any{"body": map[string]any{"type": "integer"}}},
		),
	)
	require.NoError(t, err)
	mapped, err := NewTool("mapped", "mapped", handler, WithSchemaRegistry(registry))
	require.NoError(t, err)
	// Act.
	explicitErr := tool.Execute(t.Context(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
	mappedErr := mapped.Execute(t.Context(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
	// Assert.
	require.NoError(t, explicitErr)
	requireToolErrorCode(t, mappedErr, CodeInternal)
	rawType := registry.buildTypeSchemas()[reflect.TypeFor[json.RawMessage]()]
	require.Equal(t, "string", rawType.Type)
	require.Len(t, registry.buildTypeSchemas(), 1)
}

func TestRawMessageDefaultsDoNotMutateSharedRegistry(t *testing.T) {
	// Arrange.
	registry := NewSchemaRegistry()
	// Act.
	tool, err := NewTool("raw", "raw", func(context.Context, *RunEnv, rawOutputFixture) (rawOutputFixture, error) {
		return rawOutputFixture{Body: json.RawMessage(`true`)}, nil
	}, WithSchemaRegistry(registry))
	// Assert.
	require.NoError(t, err)
	require.Empty(t, registry.buildTypeSchemas())
	inputBody := tool.Manifest().Parameters["properties"].(map[string]any)["body"].(map[string]any)
	require.Equal(t, "object", inputBody["type"])
	require.Equal(t, true, tool.Manifest().OutputSchema["properties"].(map[string]any)["body"])
}

func TestTopLevelRawMessageShapeRequiresExplicitSchema(t *testing.T) {
	// Arrange.
	handler := func(context.Context, *RunEnv, struct{}) (json.RawMessage, error) {
		return json.RawMessage(`[1,true]`), nil
	}
	tool, err := NewTool("raw", "raw", handler)
	require.NoError(t, err)
	constrained, err := NewTool(
		"constrained",
		"constrained",
		handler,
		WithOutputSchema(map[string]any{"type": "object"}),
	)
	require.NoError(t, err)
	// Act.
	rawErr := tool.Execute(t.Context(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil })
	shapeErr := constrained.Execute(
		t.Context(),
		nil,
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { return nil },
	)
	// Assert.
	require.Empty(t, tool.Manifest().OutputSchema)
	require.NoError(t, rawErr)
	requireToolErrorCode(t, shapeErr, CodeInternal)
}

func TestGenericResultChunksRejectPayloadFreeContradictions(t *testing.T) {
	for _, mode := range []string{
		"empty-wire", "noop-wire", "noop-effect", "both", "empty-error", "noop-error",
	} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			chunk := Chunk{Event: EventResult}
			switch mode {
			case "empty-wire":
				chunk.EmptyResult = true
				chunk.Data = []byte(`7`)
				chunk.MimeType = MimeTypeJSON
			case "noop-wire":
				chunk.Noop = true
				chunk.Data = []byte(`7`)
				chunk.MimeType = MimeTypeJSON
			case "noop-effect":
				chunk.Noop = true
				chunk.Effects = []any{"recorded"}
			case "both":
				chunk.EmptyResult = true
				chunk.Noop = true
			case "empty-error":
				chunk = NewErrorChunkFromErr(NewValidationError("bad"))
				chunk.EmptyResult = true
			case "noop-error":
				chunk = NewErrorChunkFromErr(NewValidationError("bad"))
				chunk.Noop = true
			}
			// Act.
			_, err := prepareResultChunk(chunk, nil)
			// Assert.
			requireToolErrorCode(t, err, CodeInternal)
			var contract *ResultContractError
			require.ErrorAs(t, err, &contract)
			require.Equal(t, "result_algebra", contract.Kind)
		})
	}
}

type resultCloneProbe struct {
	Exported []int
	hidden   []int
	Next     *resultCloneProbe
}

func TestResultCloneOwnershipLimits(t *testing.T) {
	// Arrange.
	original := &resultCloneProbe{Exported: []int{1}, hidden: []int{2}}
	original.Next = original
	key := &resultCloneProbe{}
	keyed := map[*resultCloneProbe]int{key: 7}
	backing := []int{3, 4, 5}
	overlapping := [][]int{backing[:1], backing[:2]}
	// Act.
	cloned := cloneMutableValue(original).(*resultCloneProbe)
	clonedKeys := cloneMutableValue(keyed).(map[*resultCloneProbe]int)
	clonedSlices := cloneMutableValue(overlapping).([][]int)
	cloned.Exported[0] = 8
	cloned.hidden[0] = 9
	clonedSlices[0][0] = 10
	// Assert.
	require.NotSame(t, original, cloned)
	require.Same(t, cloned, cloned.Next)
	require.Equal(t, 1, original.Exported[0])
	require.Equal(t, 9, original.hidden[0])
	require.Equal(t, 7, clonedKeys[key])
	require.Equal(t, 3, clonedSlices[1][0])
	require.Equal(t, 3, backing[0])
	require.Equal(t, len(clonedSlices[0]), cap(clonedSlices[0]))
}

func assertExactOutputSchema[R any](t *testing.T, value R, want string) {
	t.Helper()
	// Arrange.
	tool, err := NewTool("schema", "schema", func(context.Context, *RunEnv, struct{}) (R, error) { return value, nil })
	require.NoError(t, err)
	// Act.
	raw, err := json.Marshal(tool.Manifest().OutputSchema)
	require.NoError(t, err)
	// Assert.
	require.JSONEq(t, want, string(raw))
	require.NoError(
		t,
		tool.Execute(t.Context(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error { return nil }),
	)
}

func TestExactGeneratedOutputSchemas(t *testing.T) {
	t.Run("integer", func(t *testing.T) { assertExactOutputSchema(t, 7, `{"type":"integer"}`) })
	t.Run("string", func(t *testing.T) { assertExactOutputSchema(t, "value", `{"type":"string"}`) })
	t.Run("boolean", func(t *testing.T) { assertExactOutputSchema(t, true, `{"type":"boolean"}`) })
	t.Run("array", func(t *testing.T) {
		assertExactOutputSchema(t, []int{1}, `{"type":["null","array"],"items":{"type":"integer"}}`)
	})
	t.Run("raw-member", func(t *testing.T) {
		assertExactOutputSchema(
			t,
			rawOutputFixture{Body: json.RawMessage(`true`)},
			`{"type":"object","properties":{"body":true},"required":["body"],"additionalProperties":false}`,
		)
	})
}

func TestEmptyResultRetainsOpaqueValueWithoutSerialization(t *testing.T) {
	for _, noop := range []bool{false, true} {
		t.Run(strconv.FormatBool(noop), func(t *testing.T) {
			// Arrange.
			opaque := map[string]any{"function": func() {}}
			tool, err := NewTypedTool(
				TypedToolSpec[NoSubject, NoScope, struct{}, map[string]any, string]{
					Name:        "opaque",
					Description: "opaque",
					Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[map[string]any, string], error) {
						result := NewToolResult[map[string]any, string](opaque)
						result.Noop = noop
						result.Empty = !noop
						return result, nil
					},
				},
			)
			require.NoError(t, err)
			var delivered Chunk
			// Act.
			err = tool.Execute(
				t.Context(),
				nil,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(c Chunk) error { delivered = c; return nil },
			)
			// Assert: host codecs own persistence of this non-JSON value.
			require.NoError(t, err)
			require.Empty(t, delivered.Data)
			require.IsType(t, (func())(nil), delivered.TypedResult.(map[string]any)["function"])
			require.IsType(t, (func())(nil), delivered.Envelope.Result.(map[string]any)["function"])
			_, codecErr := (JSONResultCodec[map[string]any, string]{}).EncodeResult(delivered)
			require.Error(t, codecErr)
		})
	}
}

func TestEmptyGenericResultRetainsProtocolValue(t *testing.T) {
	// Arrange: MCP/read adapters may have an empty text projection and a full typed response.
	value := map[string]any{"contents": []any{}, "_meta": map[string]any{"source": "protocol"}}
	chunk := Chunk{
		Event:       EventResult,
		EmptyResult: true,
		TypedResult: value,
		Envelope: NewResultEnvelope(
			value,
			nil,
			"",
			DeliveryClassText,
			AudienceInternal,
			map[string]any{"trace": "retained"},
		),
	}
	// Act.
	prepared, err := prepareResultChunk(chunk, nil)
	// Assert.
	require.NoError(t, err)
	require.Empty(t, prepared.Data)
	require.Equal(t, value, prepared.TypedResult)
	require.Equal(t, value, prepared.Envelope.Result)
	require.Equal(t, AudienceInternal, prepared.Envelope.Audience)
}
