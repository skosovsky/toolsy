package toolsyotel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/toolsy/history"
)

func TestRecordSemanticTruncation_EmitsSpanWithAttributes(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	report := history.SemanticTruncationReport{
		Applied:                  true,
		SummarizationApplied:     true,
		MechanicalTruncationUsed: false,
		FallbackUsed:             false,
		SummarizerFailed:         false,
		TokensBefore:             1200,
		TokensAfter:              800,
		MessagesCompressedCount:  5,
	}

	RecordSemanticTruncation(context.Background(), report, nil, WithTracerProvider(tp))

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "toolsy.semantic_truncation", span.Name())
	assert.Equal(t, codes.Unset, span.Status().Code)

	before, ok := attrValue(span, "toolsy.truncation.tokens_before")
	require.True(t, ok)
	assert.Equal(t, int64(1200), before.AsInt64())

	after, ok := attrValue(span, "toolsy.truncation.tokens_after")
	require.True(t, ok)
	assert.Equal(t, int64(800), after.AsInt64())

	compressed, ok := attrValue(span, "toolsy.truncation.messages_compressed_count")
	require.True(t, ok)
	assert.Equal(t, int64(5), compressed.AsInt64())

	fallback, ok := attrValue(span, "toolsy.truncation.fallback_used")
	require.True(t, ok)
	assert.False(t, fallback.AsBool())
}

func TestRecordSemanticTruncation_MarksSpanErrorOnSummarizerFailure(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	report := history.SemanticTruncationReport{
		SummarizerFailed: true,
		FallbackUsed:     true,
	}

	RecordSemanticTruncation(context.Background(), report, nil, WithTracerProvider(tp))

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
}

func TestRecordSemanticTruncation_RecordsExecError(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	report := history.SemanticTruncationReport{}
	execErr := errors.New("count tokens failed")

	RecordSemanticTruncation(context.Background(), report, execErr, WithTracerProvider(tp))

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.NotEmpty(t, span.Events())
}

func TestRecordSemanticTruncation_UsesDefaultTracerProvider(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	RecordSemanticTruncation(
		context.Background(),
		history.SemanticTruncationReport{TokensBefore: 10, TokensAfter: 5},
		nil,
		WithTracerProvider(tp),
	)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "toolsy.semantic_truncation", spans[0].Name())
}
