package toolsyotel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/toolsy"
)

type stubTool struct {
	manifest toolsy.ToolManifest
	execute  func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error
}

func (s *stubTool) Manifest() toolsy.ToolManifest { return s.manifest }

func (s *stubTool) Execute(
	ctx context.Context,
	run toolsy.RunContext,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if s.execute != nil {
		return s.execute(ctx, run, input, yield)
	}
	return nil
}

func newSpanRecorder() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return tp, rec
}

func attrValue(span sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func hasEvent(span sdktrace.ReadOnlySpan, name string) bool {
	for _, ev := range span.Events() {
		if ev.Name == name {
			return true
		}
	}
	return false
}

func TestWithTracing_SetsSpanNameAndAttributes(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "weather"},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(
		context.Background(),
		toolsy.RunContext{},
		toolsy.ToolInput{CallID: "call-1", ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "tool.execute.weather", span.Name())
	assert.Equal(t, codes.Unset, span.Status().Code)

	toolName, ok := attrValue(span, "gen_ai.tool.name")
	require.True(t, ok)
	assert.Equal(t, "weather", toolName.AsString())

	callID, ok := attrValue(span, "gen_ai.tool.call_id")
	require.True(t, ok)
	assert.Equal(t, "call-1", callID.AsString())
}

func TestWithTracing_ErrorMarksSpan(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "db_tool"},
		execute: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return errors.New("db down")
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.Error(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
}

func TestWithTracing_SuspendIsNeutral(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "human_approval"},
		execute: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return toolsy.ErrSuspend
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, toolsy.ErrSuspend)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, codes.Unset, span.Status().Code)
	assert.True(t, hasEvent(span, "tool.suspend"))

	suspended, ok := attrValue(span, "gen_ai.tool.suspended")
	require.True(t, ok)
	assert.True(t, suspended.AsBool())
}

func TestWithTracing_StreamAbortedIsNeutral(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "stream_tool"},
		execute: func(context.Context, toolsy.RunContext, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return toolsy.ErrStreamAborted
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.RunContext{}, toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, toolsy.ErrStreamAborted)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, codes.Unset, span.Status().Code)
	assert.True(t, hasEvent(span, "tool.stream_aborted"))

	aborted, ok := attrValue(span, "gen_ai.tool.stream_aborted")
	require.True(t, ok)
	assert.True(t, aborted.AsBool())
}
