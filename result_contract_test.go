package toolsy

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resultContractBuilders(t *testing.T, schema map[string]any, value int) map[string]Tool {
	t.Helper()
	opts := []ToolOption{WithOutputSchema(schema)}
	typed, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, int, string]{
		Name:        "typed",
		Description: "Typed",
		Options:     opts,
		Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[int, string], error) {
			return NewToolResult[int, string](value), nil
		},
	})
	require.NoError(t, err)
	generic, err := NewTool("generic", "Generic", func(context.Context, *RunEnv, struct{}) (int, error) {
		return value, nil
	}, opts...)
	require.NoError(t, err)
	produce := func(y func(Chunk) error) error {
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		// Deliberately ignore a rejection to exercise the sticky producer boundary.
		_ = y(Chunk{Event: EventResult, Data: raw, MimeType: MimeTypeJSON})
		return nil
	}
	proxy, err := NewProxyTool("proxy", "Proxy", []byte(`{"type":"object"}`),
		func(_ context.Context, _ *RunEnv, _ []byte, y func(Chunk) error) error { return produce(y) }, opts...)
	require.NoError(t, err)
	dynamic, err := NewDynamicToolFromSpec(DynamicToolSpec{
		Name: "dynamic", Description: "Dynamic", Schema: MapSchemaProvider{"type": "object"}, Options: opts,
		Handler: func(_ context.Context, _ *RunEnv, _ map[string]any, y func(Chunk) error) error { return produce(y) },
	})
	require.NoError(t, err)
	stream, err := NewStreamTool("stream", "Stream",
		func(_ context.Context, _ *RunEnv, _ struct{}, y func(Chunk) error) error { return produce(y) },
		WithIndependentStream(), WithOutputSchema(schema))
	require.NoError(t, err)
	return map[string]Tool{
		"typed":       typed,
		"generic":     generic,
		"proxy":       proxy,
		"dynamic":     dynamic,
		"independent": stream,
	}
}

func TestResultContractNonBusinessOutputs(t *testing.T) {
	for _, c := range []Chunk{
		{Event: EventResult, Data: []byte{0xff, 0}, MimeType: MimeTypeOctetStream},
		{Event: EventProgress, Data: []byte(`"progress"`), MimeType: MimeTypeJSON},
		{Event: EventControl, Control: &PauseSignal{Reason: "input"}},
		NewErrorChunkFromErr(NewValidationError("business failure")),
	} {
		// Arrange: none of these payloads is a successful JSON business value.
		tool, err := NewProxyTool("non-business", "Non business", []byte(`{"type":"object"}`),
			func(_ context.Context, _ *RunEnv, _ []byte, y func(Chunk) error) error { return y(c) },
			WithOutputSchema(map[string]any{"type": "integer", "minimum": 1}))
		require.NoError(t, err)
		var delivered []Chunk
		// Act.
		err = tool.Execute(context.Background(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
			delivered = append(delivered, c)
			return nil
		})
		// Assert.
		require.NoError(t, err)
		require.Len(t, delivered, 1)
		assert.Equal(t, c.Data, delivered[0].Data)
	}
}

func TestResultContractWireFormatterSchema(t *testing.T) {
	for _, raw := range []string{`{"count":-1}`, `{"count":1}`} {
		// Arrange: the wrapper type has no relation to the encoded domain shape.
		tool, err := NewTool("formatted", "Formatted", func(context.Context, *RunEnv, struct{}) (wireJSONStub, error) {
			return wireJSONStub{raw: json.RawMessage(raw)}, nil
		}, WithOutputSchema(map[string]any{"type": "object", "properties": map[string]any{
			"count": map[string]any{"type": "integer", "minimum": 1},
		}, "required": []string{"count"}}))
		require.NoError(t, err)
		count := 0
		// Act.
		err = tool.Execute(
			context.Background(),
			nil,
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { count++; return nil },
		)
		// Assert.
		if raw == `{"count":-1}` {
			require.Error(t, err)
			assert.Zero(t, count)
		} else {
			require.NoError(t, err)
			assert.Equal(t, 1, count)
		}
	}
}

func TestResultContractRejectedOutputIsNotCached(t *testing.T) {
	// Arrange: use the actual cache profile, not a capture-only test double.
	store := NewMemoryResultCacheStore()
	cache, err := NewResultCache(store, constantPartition, JSONResultCodec[int, string]{}, 0)
	require.NoError(t, err)
	tool, err := NewTool("bad", "Bad", func(context.Context, *RunEnv, struct{}) (int, error) { return -1, nil },
		WithIdempotent(), WithOutputSchema(map[string]any{"type": "integer", "minimum": 1}))
	require.NoError(t, err)
	// Act.
	err = tool.Execute(
		context.Background(),
		NewRunEnv(nil, WithRunExecutionProfile(cache)),
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(Chunk) error { t.Fatal("invalid result delivered"); return nil },
	)
	// Assert.
	require.Error(t, err)
	assert.Empty(t, store.items)
}

func TestResultContractConcurrentRejectionAndEnvelope(t *testing.T) {
	for _, mode := range []string{"concurrent", "envelope", "error-on-success", "missing-error"} {
		// Arrange: malformed yields are deliberately ignored by a producer.
		tool, err := NewProxyTool("adversarial", "Adversarial", []byte(`{"type":"object"}`),
			func(_ context.Context, _ *RunEnv, _ []byte, y func(Chunk) error) error {
				if mode == "error-on-success" {
					envelope := NewResultEnvelope(nil, []byte(`1`), MimeTypeJSON, "", "", nil)
					envelope.Error = NewValidationError("contradictory classification")
					_ = y(Chunk{Event: EventResult, Data: []byte(`1`), MimeType: MimeTypeJSON, Envelope: envelope})
					return nil
				}
				if mode == "missing-error" {
					c := NewErrorChunkFromErr(NewValidationError("business error"))
					c.Envelope.Error = nil
					_ = y(c)
					return nil
				}
				if mode == "envelope" {
					_ = y(Chunk{Event: EventResult, Data: []byte(`1`), MimeType: MimeTypeJSON,
						Envelope: NewResultEnvelope(nil, []byte(`-1`), MimeTypeJSON, "", "", nil)})
					return nil
				}
				var workers sync.WaitGroup
				for range 8 {
					workers.Go(
						func() { _ = y(Chunk{Event: EventResult, Data: []byte(`{"invalid`), MimeType: MimeTypeJSON}) },
					)
				}
				workers.Wait()
				return nil
			}, WithOutputSchema(map[string]any{"type": "integer", "minimum": 1}))
		require.NoError(t, err)
		// Act.
		err = tool.Execute(context.Background(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(Chunk) error {
			t.Error("unchecked result delivered")
			return nil
		})
		// Assert.
		var contract *ResultContractError
		require.ErrorAs(t, err, &contract)
	}
}

func TestResultContractAllBuilders(t *testing.T) {
	for _, value := range []int{-1, 1} {
		// Arrange: the same constraint applies to every builder's wire value.
		for name, tool := range resultContractBuilders(t, map[string]any{"type": "integer", "minimum": 1}, value) {
			t.Run(name+"/"+strconv.Itoa(value), func(t *testing.T) {
				var delivered []Chunk
				// Act.
				err := tool.Execute(context.Background(), nil, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
					delivered = append(delivered, c)
					return nil
				})
				// Assert.
				if value < 0 {
					var contract *ResultContractError
					require.ErrorAs(t, err, &contract)
					assert.Equal(t, "schema_mismatch", contract.Kind)
					assert.Empty(t, delivered)
					te, ok := AsToolError(err)
					require.True(t, ok)
					assert.False(t, te.Retryable)
				} else {
					require.NoError(t, err)
					require.Len(t, delivered, 1)
					assert.JSONEq(t, `1`, string(delivered[0].Data))
				}
			})
		}
	}
}

func TestResultContractInvalidSchemaRejectedAtConstruction(t *testing.T) {
	// Arrange: invalid output schemas fail before a handler can dispatch.
	called := false
	opts := []ToolOption{WithOutputSchema(map[string]any{"type": "invalid"})}
	// Act.
	_, genericErr := NewTool("invalid", "Invalid", func(context.Context, *RunEnv, struct{}) (int, error) {
		called = true
		return 1, nil
	}, opts...)
	_, proxyErr := NewProxyTool("invalid", "Invalid", []byte(`{"type":"object"}`),
		func(context.Context, *RunEnv, []byte, func(Chunk) error) error { called = true; return nil }, opts...)
	_, typedErr := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, int, string]{
		Name:        "invalid",
		Description: "Invalid",
		Options:     opts,
		Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[int, string], error) {
			called = true
			return NewToolResult[int, string](1), nil
		},
	})
	_, streamErr := NewStreamTool("invalid", "Invalid",
		func(context.Context, *RunEnv, struct{}, func(Chunk) error) error { called = true; return nil },
		WithIndependentStream(), opts[0])
	_, dynamicErr := NewDynamicToolFromSpec(DynamicToolSpec{
		Name: "invalid", Description: "Invalid", Schema: MapSchemaProvider{"type": "object"}, Options: opts,
		Handler: func(context.Context, *RunEnv, map[string]any, func(Chunk) error) error { called = true; return nil },
	})
	// Assert.
	for _, err := range []error{genericErr, proxyErr, typedErr, streamErr, dynamicErr} {
		require.Error(t, err)
	}
	assert.False(t, called)
}

func TestResultContractRepresentation(t *testing.T) {
	for _, mode := range []string{"raw-json", "raw-text", "empty", "noop"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: typed values remain host-owned; JSON schema applies to wire bytes.
			tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, int, string]{
				Name:        "representation",
				Description: "Representation",
				Options:     []ToolOption{WithOutputSchema(map[string]any{"type": "integer", "minimum": 1})},
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[int, string], error) {
					result := NewToolResult[int, string](-1)
					switch mode {
					case "raw-json":
						result.Raw, result.RawMimeType = []byte(`1`), MimeTypeJSON
					case "raw-text":
						result.Raw, result.RawMimeType = []byte("not JSON"), MimeTypeText
					case "empty":
						result = NewEmptyToolResult[int, string]()
					case "noop":
						result = NewNoopToolResult[int, string]()
					}
					return result, nil
				},
			})
			require.NoError(t, err)
			var delivered Chunk
			// Act.
			err = tool.Execute(
				context.Background(),
				nil,
				ToolInput{ArgsJSON: []byte(`{}`)},
				func(c Chunk) error { delivered = c; return nil },
			)
			// Assert.
			require.NoError(t, err)
			if mode == "raw-json" || mode == "raw-text" {
				assert.Equal(t, -1, delivered.TypedResult)
			}
		})
	}
}

func TestResultContractBeforeProfileAndOnReplay(t *testing.T) {
	for _, replay := range []bool{false, true} {
		// Arrange: profile either observes the invocation or supplies an invalid replay.
		tool := resultContractBuilders(t, map[string]any{"type": "integer", "minimum": 1}, -1)["proxy"]
		captured, delivered := 0, 0
		profile := executionProfileFunc(
			func(_ context.Context, _ PreparedCall, invoke InvocationHandler, y func(Chunk) error) error {
				if replay {
					return y(Chunk{Event: EventResult, Data: []byte(`-1`), MimeType: MimeTypeJSON})
				}
				return invoke(func(c Chunk) error { captured++; return y(c) })
			},
		)
		// Act.
		err := tool.Execute(
			context.Background(),
			NewRunEnv(nil, WithRunExecutionProfile(profile)),
			ToolInput{ArgsJSON: []byte(`{}`)},
			func(Chunk) error { delivered++; return nil },
		)
		// Assert: neither persistence input nor delivery receives invalid success.
		var contract *ResultContractError
		require.ErrorAs(t, err, &contract)
		assert.Zero(t, captured)
		assert.Zero(t, delivered)
	}
}

func TestResultContractOperationFailureRemainsUnknown(t *testing.T) {
	// Arrange: output validation fails after the external action.
	effects := 0
	tool, err := NewTool("write", "Write", func(context.Context, *RunEnv, struct{}) (int, error) {
		effects++
		return -1, nil
	}, WithOutputSchema(map[string]any{"type": "integer", "minimum": 1}))
	require.NoError(t, err)
	execute := compositionExecutor(t, tool, compositionProfile(t, "operation"), "session")
	call := ToolCall{ToolName: "write", Input: ToolInput{CallID: "first", ArgsJSON: []byte(`{}`)}}
	// Act.
	err = execute(context.Background(), call, func(Chunk) error { t.Fatal("invalid result delivered"); return nil })
	var unknown *OperationOutcomeError
	require.ErrorAs(t, err, &unknown)
	call.Input.CallID = "repeat"
	err = execute(context.Background(), call, func(Chunk) error { t.Fatal("invalid replay delivered"); return nil })
	// Assert.
	var state *OperationStateError
	require.ErrorAs(t, err, &state)
	assert.Equal(t, OperationUnknown, state.Record.State)
	assert.Equal(t, 1, effects)
}
