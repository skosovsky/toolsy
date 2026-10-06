package toolsy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numericInputSchema(rule map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"id": rule}, "required": []any{"id"}}
}

func TestLosslessDynamicInputIdentity(t *testing.T) {
	// Arrange.
	var received []string
	var snapshots []string
	var keys []string
	cache, err := NewResultCache(
		NewMemoryResultCacheStore(),
		func(context.Context, PreparedCall) (string, error) { return "test", nil },
		JSONResultCodec[any, any]{},
		0,
	)
	require.NoError(t, err)
	profile := executionProfileFunc(
		func(ctx context.Context, call PreparedCall, invoke InvocationHandler, yield func(Chunk) error) error {
			snapshots = append(snapshots, string(call.Input.ArgsJSON))
			key, keyErr := cache.key(ctx, call)
			if keyErr != nil {
				return keyErr
			}
			keys = append(keys, key)
			return invoke(yield)
		},
	)
	tool, err := newDynamicTool(
		"numeric",
		"Numeric",
		numericInputSchema(map[string]any{"type": "integer"}),
		func(_ context.Context, _ *RunEnv, args map[string]any, _ func(Chunk) error) error {
			received = append(received, fmt.Sprint(args["id"]))
			return nil
		},
	)
	require.NoError(t, err)
	values := []string{"9007199254740991", "9007199254740992", "9007199254740993", "9223372036854775807"}
	// Act.
	for _, value := range values {
		require.NoError(
			t,
			tool.Execute(
				context.Background(),
				NewRunEnv(nil, WithRunExecutionProfile(profile)),
				ToolInput{ArgsJSON: []byte(`{"id":` + value + `}`)},
				func(Chunk) error { return nil },
			),
		)
	}
	// Assert.
	assert.Equal(t, values, received)
	for i, value := range values {
		assert.Equal(t, `{"id":`+value+`}`, snapshots[i])
	}
	require.Len(t, map[string]bool{keys[0]: true, keys[1]: true, keys[2]: true, keys[3]: true}, 4)
}

func TestLosslessNumericSchemaConstraints(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rule      map[string]any
		good, bad string
	}{
		{"minimum", map[string]any{"type": "integer", "minimum": json.Number("9007199254740993")}, "9007199254740993", "9007199254740992"},
		{"maximum", map[string]any{"type": "integer", "maximum": json.Number("9007199254740992")}, "9007199254740992", "9007199254740993"},
		{"enum", map[string]any{"type": "integer", "enum": []any{json.Number("9007199254740993")}}, "9007199254740993", "9007199254740992"},
		{"fractional", map[string]any{"type": "integer"}, "9007199254740993", "9007199254740993.5"},
		{"int64", map[string]any{"type": "integer", "maximum": json.Number("9223372036854775807")}, "9223372036854775807", "9223372036854775808"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			calls := 0
			tool, err := newDynamicTool(
				"bound",
				"Bound",
				numericInputSchema(tc.rule),
				func(context.Context, *RunEnv, map[string]any, func(Chunk) error) error { calls++; return nil },
			)
			require.NoError(t, err)
			// Act.
			goodErr := tool.Execute(
				context.Background(),
				NewRunEnv(nil),
				ToolInput{ArgsJSON: []byte(`{"id":` + tc.good + `}`)},
				func(Chunk) error { return nil },
			)
			badErr := tool.Execute(
				context.Background(),
				NewRunEnv(nil),
				ToolInput{ArgsJSON: []byte(`{"id":` + tc.bad + `}`)},
				func(Chunk) error { return nil },
			)
			// Assert.
			require.NoError(t, goodErr)
			require.Error(t, badErr)
			require.Equal(t, 1, calls)
		})
	}
}

func TestLosslessTypedExtractorValidation(t *testing.T) {
	// Arrange: exercise both schema validation and typed decoding at the same boundary.
	type args struct {
		ID int64 `json:"id"`
	}
	ext, err := NewExtractor[args](false)
	require.NoError(t, err)
	ext.resolved, err = compileRawSchema(
		numericInputSchema(
			map[string]any{
				"type": "integer",
				"enum": []any{json.Number("9007199254740993"), json.Number("9223372036854775807")},
			},
		),
	)
	require.NoError(t, err)
	// Act.
	got, goodErr := ext.ParseAndValidate([]byte(`{"id":9007199254740993}`))
	maxValue, maxErr := ext.ParseAndValidate([]byte(`{"id":9223372036854775807}`))
	_, badErr := ext.ParseAndValidate([]byte(`{"id":9007199254740992}`))
	// Assert.
	require.NoError(t, goodErr)
	require.Equal(t, int64(9007199254740993), got.ID)
	require.NoError(t, maxErr)
	require.Equal(t, int64(9223372036854775807), maxValue.ID)
	require.Error(t, badErr)
}

func TestLosslessInputStructurePolicy(t *testing.T) {
	for _, raw := range []string{`{"id":1,"id":2}`, `{"nested":{"id":1,"id":2}}`, `{"id":1} {}`, `{"nested":` + strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130) + "}"} {
		t.Run(fmt.Sprintf("case-%d", len(raw)), func(t *testing.T) {
			// Arrange.
			calls := 0
			tool, err := newDynamicTool(
				"structure",
				"Structure",
				map[string]any{"type": "object"},
				func(context.Context, *RunEnv, map[string]any, func(Chunk) error) error { calls++; return nil },
			)
			require.NoError(t, err)
			ext, err := NewExtractor[map[string]any](false)
			require.NoError(t, err)
			// Act.
			dynamicErr := tool.Execute(
				context.Background(),
				NewRunEnv(nil),
				ToolInput{ArgsJSON: []byte(raw)},
				func(Chunk) error { return nil },
			)
			_, typedErr := ext.ParseAndValidate([]byte(raw))
			// Assert.
			require.Error(t, dynamicErr)
			require.Error(t, typedErr)
			require.Zero(t, calls)
		})
	}
}

func TestLosslessTypedInterfaceNumbers(t *testing.T) {
	// Arrange.
	type args struct {
		Value any `json:"value"`
	}
	ext, err := NewExtractor[args](false)
	require.NoError(t, err)
	// Act.
	got, err := ext.ParseAndValidate([]byte(`{"value":9007199254740993}`))
	// Assert.
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), got.Value)
}

func TestLosslessSchemaTransformPreservesNamedPropertiesAndLiterals(t *testing.T) {
	// Arrange: property names and const object keys are data, not schema keywords.
	literal := map[string]any{"id": "keep", "properties": "keep", "$id": "keep"}
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"id":         map[string]any{"type": "integer"},
		"properties": map[string]any{"type": "object", "const": literal},
	}, "$id": "remove"}
	// Act.
	stripSchemaIDs(schema)
	applyStrictMode(schema)
	// Assert.
	require.NotContains(t, schema, "$id")
	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "id")
	require.Contains(t, props, "properties")
	require.Equal(t, map[string]any{"id": "keep", "properties": "keep", "$id": "keep"}, literal)
	require.Equal(t, []any{"id", "properties"}, schema["required"])
}

func TestLosslessInputStructureLimits(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		allowed   bool
	}{
		{"depth-at-limit", `{"value":` + strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127) + "}", true},
		{"depth-over-limit", `{"value":` + strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128) + "}", false},
		{"nodes-at-limit", `{"value":[` + strings.Repeat("0,", 99997) + "0]}", true},
		{"nodes-over-limit", `{"value":[` + strings.Repeat("0,", 99998) + "0]}", false},
		{"escaped-duplicate", `{"value":1,"\u0076alue":2}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			ext, err := NewExtractor[map[string]any](false)
			require.NoError(t, err)
			// Act.
			_, err = ext.ParseAndValidate([]byte(tc.raw))
			// Assert.
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
