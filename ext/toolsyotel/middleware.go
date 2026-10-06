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

func (s *softErrorState) recordFromChunk(c toolsy.Chunk, text string) {
	if !c.IsError {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flag = true
	if text != "" {
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
) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.tool.name", toolName),
		attribute.String("gen_ai.operation.name", "execute_tool"),
	}
	if t.cfg.langfuseCompatibility {
		attrs = append(attrs, attribute.String("langfuse.observation.type", "tool"))
	}
	if input.CallID != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.call.id", input.CallID))
	}
	if !t.cfg.contentCapture {
		return attrs
	}
	argsText := t.cfg.captured(ContentInput, string(input.ArgsJSON))
	attrs = append(attrs, attribute.String("gen_ai.tool.call.arguments", argsText))
	if t.cfg.langfuseCompatibility {
		attrs = append(attrs, attribute.String("langfuse.observation.input", argsText))
	}
	return attrs
}

func (t *tracingTool) finalizeExecuteSpan(
	span trace.Span,
	execErr error,
	outAcc *payloadAccumulator,
	soft *softErrorState,
) {
	hasSoftError, softText := soft.snapshot()
	t.setOutputAttributes(span, execErr, outAcc, hasSoftError)
	t.applySpanStatusFromExec(span, execErr, hasSoftError, softText)
}

func (t *tracingTool) applySpanStatusFromExec(span trace.Span, execErr error, hasSoftError bool, softText string) {
	switch {
	case execErr == nil && hasSoftError:
		span.SetAttributes(attribute.Bool("toolsy.tool.soft_error", true))
		if softText != "" {
			span.SetAttributes(attribute.String("toolsy.tool.soft_error_text", softText))
		}
		span.AddEvent("tool.soft_error")
		span.SetStatus(codes.Error, "tool returned soft error chunk")
	case execErr == nil:
	case toolsy.IsControlError(execErr):
		span.SetAttributes(attribute.Bool("toolsy.tool.control_signal", true))
		span.AddEvent("tool.control")
	case errors.Is(execErr, toolsy.ErrStreamAborted):
		span.SetAttributes(attribute.Bool("toolsy.tool.stream_aborted", true))
		span.AddEvent("tool.stream_aborted")
	default:
		t.cfg.recordFailure(span, execErr, ContentError, "tool execution failed")
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
		var text string
		if t.cfg.contentCapture {
			if c.IsError {
				text = t.cfg.captured(ContentError, toolsy.ErrorChunkSummaryText(c, nil))
			} else {
				text = t.cfg.captured(ContentOutput, chunkPayloadText(c))
			}
		}
		soft.recordFromChunk(c, text)
		if outAcc != nil {
			outAcc.append(text)
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
		trace.WithAttributes(t.spanStartAttributes(toolName, input)...),
	)
	var execErr error
	defer func() {
		if p := recover(); p != nil {
			t.cfg.recordFailure(span, p, ContentPanic, "tool execution panicked")
			// End before rethrowing: the OTel SDK End implementation observes
			// active panics and otherwise records their raw values automatically.
			span.End()
			panic(p)
		}
		t.finalizeExecuteSpan(span, execErr, outAcc, &soft)
		span.End()
	}()

	execErr = t.next.Execute(ctx, run, input, t.wrapYield(yield, &soft, outAcc))
	return execErr
}

func (t *tracingTool) setOutputAttributes(
	span trace.Span,
	execErr error,
	outAcc *payloadAccumulator,
	hasSoftError bool,
) {
	if !t.cfg.contentCapture {
		return
	}
	var output string
	var attrs []attribute.KeyValue
	if execErr != nil {
		output = t.cfg.captured(ContentError, execErr.Error())
		attrs = append(attrs, attribute.String("toolsy.tool.error", output))
	} else if outAcc != nil {
		output = outAcc.String()
		attrs = append(attrs, attribute.String("toolsy.tool.output", output))
		if !hasSoftError {
			attrs = append(attrs, attribute.String("gen_ai.tool.call.result", output))
		}
	}
	if t.cfg.langfuseCompatibility {
		attrs = append(attrs, attribute.String("langfuse.observation.output", output))
	}
	span.SetAttributes(attrs...)
}

var _ toolsy.Tool = (*tracingTool)(nil)

// recordFailure deliberately avoids RecordError: it serializes arbitrary error
// text into exception.message regardless of the capture policy.
func (c config) recordFailure(span trace.Span, failure any, kind ContentKind, status string) {
	attrs := []attribute.KeyValue{
		attribute.String("exception.type", truncatePayload(fmt.Sprintf("%T", failure), c.effectiveMaxPayloadSize())),
	}
	if c.contentCapture {
		var text string
		if err, ok := failure.(error); ok {
			text = err.Error()
		} else {
			text = fmt.Sprint(failure)
		}
		attrs = append(attrs, attribute.String("exception.message", c.captured(kind, text)))
	}
	span.AddEvent("exception", trace.WithAttributes(attrs...))
	span.SetStatus(codes.Error, status)
}
