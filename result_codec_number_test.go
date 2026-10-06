package toolsy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONResultCodecPreservesDynamicNumbers(t *testing.T) {
	// Arrange: exact integers, decimal and exponent lexemes nested across the outcome.
	values := map[string]any{
		"large":  json.Number("9007199254740993"),
		"nested": []any{json.Number("0.123456789012345678901"), json.Number("1.2300e+40")},
	}
	data, err := json.Marshal(values)
	require.NoError(t, err)
	chunk := Chunk{Event: EventResult, Data: data, MimeType: MimeTypeJSON, TypedResult: values,
		Effects: []any{values}, Envelope: NewResultEnvelope(values, data, MimeTypeJSON, DeliveryClassStructured,
			AudienceInternal, values)}
	codec := JSONResultCodec[map[string]any, map[string]any]{}
	// Act.
	raw, err := codec.EncodeResult(chunk)
	require.NoError(t, err)
	decoded, err := codec.DecodeResult(raw)
	// Assert: raw and every JSON-shaped typed field agree exactly, not approximately.
	require.NoError(t, err)
	assert.Equal(t, data, decoded.Data)
	assert.Equal(t, values, decoded.TypedResult)
	assert.Equal(t, values, decoded.ToolEnvelope().Result)
	assert.Equal(t, values, decoded.Effects[0])
	assert.Equal(t, values, decoded.ToolEnvelope().Metadata)
}

func TestJSONResultCodecRejectsTrailingDocument(t *testing.T) {
	// Arrange.
	codec := JSONResultCodec[int64, string]{}
	chunk := Chunk{Event: EventResult, Data: []byte(`9007199254740993`), MimeType: MimeTypeJSON,
		TypedResult: int64(9007199254740993)}
	raw, err := codec.EncodeResult(chunk)
	require.NoError(t, err)
	// Act.
	decoded, decodeErr := codec.DecodeResult(raw)
	var suffixErrors []error
	for _, suffix := range []string{` {}`, ` garbage`, ` null`} {
		_, err = codec.DecodeResult(append(append([]byte(nil), raw...), suffix...))
		suffixErrors = append(suffixErrors, err)
	}
	// Assert: concrete types remain concrete; trailing documents and malformed suffixes fail.
	require.NoError(t, decodeErr)
	assert.Equal(t, int64(9007199254740993), decoded.TypedResult)
	for _, suffixErr := range suffixErrors {
		require.Error(t, suffixErr)
	}
}

func numberReplayProfile(t *testing.T, kind string) ExecutionProfile {
	t.Helper()
	codec := JSONResultCodec[map[string]any, map[string]any]{}
	if kind == "cache" {
		profile, err := NewResultCache(NewMemoryResultCacheStore(), constantPartition, codec, 0)
		require.NoError(t, err)
		return profile
	}
	profile, err := NewOperationProfile(NewMemoryOperationStore(),
		func(_ context.Context, call PreparedCall) (OperationIntent, error) {
			return OperationIntent{Namespace: "number", Subject: "host", Scope: "tenant", OperationID: "intent",
				AttemptID: call.Input.CallID, PolicyFingerprint: "current", CanonicalDigest: "host",
				CanonicalRules: "json", DisplayJSON: []byte(`{}`)}, nil
		}, codec, "host", time.Now, time.Minute, 0)
	require.NoError(t, err)
	return profile
}

func TestJSONDynamicNumbersSurviveProtectedReplay(t *testing.T) {
	for _, kind := range []string{"cache", "operation"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: a normal typed tool declares a dynamic JSON result/effect type.
			calls := 0
			value := map[string]any{"number": json.Number("9007199254740993")}
			tool, err := NewTypedTool(TypedToolSpec[NoSubject, NoScope, struct{}, map[string]any, map[string]any]{
				Name:        "number",
				Description: "Exact JSON number",
				Options:     []ToolOption{WithIdempotent()},
				Handler: func(context.Context, TypedCallContext[NoSubject, NoScope], *RunEnv, ValidatedArgs[struct{}]) (ToolResult[map[string]any, map[string]any], error) {
					calls++
					return ToolResult[map[string]any, map[string]any]{Value: value, Effects: []map[string]any{value},
						EnvelopeMetadata: value, Audience: AudienceInternal}, nil
				},
			})
			require.NoError(t, err)
			execute := compositionExecutor(t, tool, numberReplayProfile(t, kind), "session")
			var chunks []Chunk
			// Act: dispatch once and replay through the same actual Session boundary.
			for _, id := range []string{"fresh", "replay"} {
				require.NoError(t, execute(context.Background(), ToolCall{ToolName: "number",
					Input: ToolInput{CallID: id, ArgsJSON: []byte(`{}`)}}, func(chunk Chunk) error {
					chunks = append(chunks, chunk)
					return nil
				}))
			}
			// Assert: typed outcome, envelope, metadata and effects all preserve the exact number.
			assert.Equal(t, 1, calls)
			require.Len(t, chunks, 2)
			for _, chunk := range chunks {
				assert.Equal(t, value, chunk.TypedResult)
				assert.Equal(t, value, chunk.ToolEnvelope().Result)
				assert.Equal(t, value, chunk.Effects[0])
				assert.Equal(t, value["number"], chunk.ToolEnvelope().Metadata["number"])
				encoded, encodeErr := json.Marshal(chunk.TypedResult)
				require.NoError(t, encodeErr)
				assert.Equal(t, string(chunk.Data), string(encoded))
			}
			assert.Equal(t, true, chunks[1].ToolEnvelope().Metadata[CacheReplayMetadata])
		})
	}
}
