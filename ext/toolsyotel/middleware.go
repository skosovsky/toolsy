package toolsyotel

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/toolsy"
)

const instrumentationName = "github.com/skosovsky/toolsy/ext/toolsyotel"

type config struct {
	tracerProvider trace.TracerProvider
}

// Option configures tracing middleware behavior.
type Option func(*config)

// WithTracerProvider sets the tracer provider used by middleware.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		if tp != nil {
			c.tracerProvider = tp
		}
	}
}

// WithTracing returns middleware that emits one span per tool execution.
// Span status is left neutral for toolsy.ErrSuspend and toolsy.ErrStreamAborted.
func WithTracing(opts ...Option) toolsy.Middleware {
	cfg := config{tracerProvider: otel.GetTracerProvider()}
	for _, opt := range opts {
		opt(&cfg)
	}
	tracer := cfg.tracerProvider.Tracer(instrumentationName)

	return func(next toolsy.Tool) toolsy.Tool {
		return &tracingTool{
			next:   next,
			tracer: tracer,
		}
	}
}

type tracingTool struct {
	next   toolsy.Tool
	tracer trace.Tracer
}

func (t *tracingTool) Manifest() toolsy.ToolManifest {
	return t.next.Manifest()
}

func (t *tracingTool) Execute(
	ctx context.Context,
	run toolsy.RunContext,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	var execErr error
	var softError bool
	var softErrorText string
	toolName := t.next.Manifest().Name
	if toolName == "" {
		toolName = "unknown"
	}

	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.tool.name", toolName),
	}
	if input.CallID != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call_id", input.CallID))
	}

	ctx, span := t.tracer.Start(
		ctx,
		"tool.execute."+toolName,
		trace.WithAttributes(attrs...),
	)
	defer func() {
		if p := recover(); p != nil {
			panicErr := fmt.Errorf("panic: %v", p)
			span.RecordError(panicErr)
			span.SetStatus(codes.Error, panicErr.Error())
			span.End()
			panic(p)
		}

		switch {
		case execErr == nil && softError:
			span.SetAttributes(attribute.Bool("gen_ai.tool.soft_error", true))
			if softErrorText != "" {
				span.SetAttributes(attribute.String("gen_ai.tool.soft_error_text", softErrorText))
			}
			span.AddEvent("tool.soft_error")
			span.SetStatus(codes.Error, "tool returned soft error chunk")
		case execErr == nil:
		case errors.Is(execErr, toolsy.ErrSuspend):
			span.SetAttributes(attribute.Bool("gen_ai.tool.suspended", true))
			span.AddEvent("tool.suspend")
		case errors.Is(execErr, toolsy.ErrStreamAborted):
			span.SetAttributes(attribute.Bool("gen_ai.tool.stream_aborted", true))
			span.AddEvent("tool.stream_aborted")
		default:
			span.RecordError(execErr)
			span.SetStatus(codes.Error, execErr.Error())
		}
		span.End()
	}()

	yieldWrapped := func(c toolsy.Chunk) error {
		if c.IsError {
			softError = true
			if c.MimeType == toolsy.MimeTypeText && utf8.Valid(c.Data) {
				softErrorText = string(c.Data)
			}
		}
		return yield(c)
	}

	execErr = t.next.Execute(ctx, run, input, yieldWrapped)
	return execErr
}

var _ toolsy.Tool = (*tracingTool)(nil)
