package toolsyotel

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/toolsy"
)

const instrumentationName = "github.com/skosovsky/toolsy/ext/toolsyotel"

// WithTracing returns middleware that emits one span per tool execution.
// Span status is left neutral for control-plane errors and toolsy.ErrStreamAborted.
func WithTracing(opts ...Option) toolsy.Middleware {
	cfg := defaultConfig()
	cfg.tracerProvider = otel.GetTracerProvider()
	for _, opt := range opts {
		opt(&cfg)
	}
	tracer := cfg.tracerProvider.Tracer(instrumentationName)

	return func(next toolsy.Tool) toolsy.Tool {
		return &tracingTool{
			next:   next,
			tracer: tracer,
			cfg:    cfg,
		}
	}
}

type tracingTool struct {
	next   toolsy.Tool
	tracer trace.Tracer
	cfg    config
}

func (t *tracingTool) Manifest() toolsy.ToolManifest {
	return t.next.Manifest()
}

func (t *tracingTool) UnwrapNext() toolsy.Tool {
	return t.next
}

var _ toolsy.ChainUnwrapper = (*tracingTool)(nil)

type softErrorState struct {
	mu   sync.Mutex
	flag bool
	text string
}

func (s *softErrorState) recordFromChunk(c toolsy.Chunk) {
	if !c.IsError {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flag = true
	if text := toolsy.ErrorChunkSummaryText(c, nil); text != "" {
		s.text = text
	}
}

func (s *softErrorState) snapshot() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flag, s.text
}

func (t *tracingTool) toolName() string {
	name := t.next.Manifest().Name
	if name == "" {
		return "unknown"
	}
	return name
}

func (t *tracingTool) spanStartAttributes(
	toolName string,
	input toolsy.ToolInput,
	maxPayload int,
) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.tool.name", toolName),
		attribute.String("gen_ai.operation.name", "execute_tool"),
		attribute.String("langfuse.observation.type", "tool"),
	}
	if input.CallID != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call_id", input.CallID))
	}
	if !t.cfg.contentCapture {
		return attrs
	}
	argsText := truncatePayload(string(input.ArgsJSON), maxPayload)
	return append(attrs,
		attribute.String("langfuse.observation.input", argsText),
		attribute.String("gen_ai.tool.call.arguments", argsText),
	)
}

func (t *tracingTool) finalizeExecuteSpan(
	span trace.Span,
	execErr error,
	outAcc *payloadAccumulator,
	maxPayload int,
	soft *softErrorState,
) {
	t.setOutputAttributes(span, execErr, outAcc, maxPayload)
	hasSoftError, softText := soft.snapshot()
	applySpanStatusFromExec(span, execErr, hasSoftError, softText)
}

func applySpanStatusFromExec(span trace.Span, execErr error, hasSoftError bool, softText string) {
	switch {
	case execErr == nil && hasSoftError:
		span.SetAttributes(attribute.Bool("gen_ai.tool.soft_error", true))
		if softText != "" {
			span.SetAttributes(attribute.String("gen_ai.tool.soft_error_text", softText))
		}
		span.AddEvent("tool.soft_error")
		span.SetStatus(codes.Error, "tool returned soft error chunk")
	case execErr == nil:
	case toolsy.IsControlError(execErr):
		span.SetAttributes(attribute.Bool("gen_ai.tool.control_signal", true))
		span.AddEvent("tool.control")
	case errors.Is(execErr, toolsy.ErrStreamAborted):
		span.SetAttributes(attribute.Bool("gen_ai.tool.stream_aborted", true))
		span.AddEvent("tool.stream_aborted")
	default:
		span.RecordError(execErr)
		span.SetStatus(codes.Error, execErr.Error())
	}
}

func (t *tracingTool) wrapYield(
	yield func(toolsy.Chunk) error,
	soft *softErrorState,
	outAcc *payloadAccumulator,
) func(toolsy.Chunk) error {
	return func(c toolsy.Chunk) error {
		if err := yield(c); err != nil {
			return err
		}
		soft.recordFromChunk(c)
		if outAcc != nil {
			if c.IsError {
				outAcc.append(toolsy.ErrorChunkSummaryText(c, nil))
			} else {
				outAcc.append(chunkPayloadText(c))
			}
		}
		return nil
	}
}

func (t *tracingTool) Execute(
	ctx context.Context,
	run *toolsy.RunEnv,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	maxPayload := t.cfg.effectiveMaxPayloadSize()
	var outAcc *payloadAccumulator
	if t.cfg.contentCapture {
		outAcc = newPayloadAccumulator(maxPayload)
	}

	toolName := t.toolName()
	var soft softErrorState

	ctx, span := t.tracer.Start(
		ctx,
		"tool.execute."+toolName,
		trace.WithAttributes(t.spanStartAttributes(toolName, input, maxPayload)...),
	)
	defer span.End()

	var execErr error
	defer func() {
		if p := recover(); p != nil {
			panicErr := fmt.Errorf("panic: %v", p)
			span.RecordError(panicErr)
			span.SetStatus(codes.Error, panicErr.Error())
			panic(p)
		}
		t.finalizeExecuteSpan(span, execErr, outAcc, maxPayload, &soft)
	}()

	execErr = t.next.Execute(ctx, run, input, t.wrapYield(yield, &soft, outAcc))
	return execErr
}

func (t *tracingTool) setOutputAttributes(span trace.Span, execErr error, outAcc *payloadAccumulator, maxPayload int) {
	if !t.cfg.contentCapture {
		return
	}
	switch {
	case execErr != nil:
		span.SetAttributes(attribute.String(
			"langfuse.observation.output",
			truncatePayload(execErr.Error(), maxPayload),
		))
	case outAcc != nil:
		output := outAcc.String()
		span.SetAttributes(
			attribute.String("langfuse.observation.output", output),
			attribute.String("gen_ai.tool.call.result", output),
		)
	}
}

var _ toolsy.Tool = (*tracingTool)(nil)
