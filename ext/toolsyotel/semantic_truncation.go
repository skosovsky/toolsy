package toolsyotel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/toolsy/history"
)

// RecordSemanticTruncation emits one semantic truncation span from report data.
// This helper keeps OTel dependencies outside the root toolsy module.
func RecordSemanticTruncation(
	ctx context.Context,
	report history.SemanticTruncationReport,
	execErr error,
	opts ...Option,
) {
	cfg := config{tracerProvider: otel.GetTracerProvider()}
	for _, opt := range opts {
		opt(&cfg)
	}
	tracer := cfg.tracerProvider.Tracer(instrumentationName)

	attrs := []attribute.KeyValue{
		attribute.Int("toolsy.truncation.tokens_before", report.TokensBefore),
		attribute.Int("toolsy.truncation.tokens_after", report.TokensAfter),
		attribute.Int("toolsy.truncation.messages_compressed_count", report.MessagesCompressedCount),
		attribute.Bool("toolsy.truncation.fallback_used", report.FallbackUsed),
		attribute.Bool("toolsy.truncation.mechanical_used", report.MechanicalTruncationUsed),
		attribute.Bool("toolsy.truncation.summarization_applied", report.SummarizationApplied),
	}

	_, span := tracer.Start(
		ctx,
		"toolsy.semantic_truncation",
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	switch {
	case execErr != nil:
		span.RecordError(execErr)
		span.SetStatus(codes.Error, execErr.Error())
	case report.SummarizerFailed:
		span.SetStatus(codes.Error, "semantic truncation summarizer failed")
	default:
	}
}
