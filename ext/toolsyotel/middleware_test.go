package toolsyotel

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	execute  func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error
}

func (s *stubTool) Manifest() toolsy.ToolManifest { return s.manifest }

func (s *stubTool) Execute(
	ctx context.Context,
	run *toolsy.RunEnv,
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

func hasAttrKey(span sdktrace.ReadOnlySpan, key string) bool {
	_, ok := attrValue(span, key)
	return ok
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
		toolsy.NewRunEnv(nil),
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
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return errors.New("db down")
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.NewRunEnv(nil), toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.Error(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
}

func TestWithTracing_ControlPauseIsNeutral(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "human_approval"},
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return toolsy.ErrPause
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.NewRunEnv(nil), toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.ErrorIs(t, err, toolsy.ErrPause)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, codes.Unset, span.Status().Code)
	assert.True(t, hasEvent(span, "tool.control"))

	control, ok := attrValue(span, "gen_ai.tool.control_signal")
	require.True(t, ok)
	assert.True(t, control.AsBool())
}

func TestWithTracing_StreamAbortedIsNeutral(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "stream_tool"},
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return toolsy.ErrStreamAborted
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.NewRunEnv(nil), toolsy.ToolInput{}, func(toolsy.Chunk) error {
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

func TestWithTracing_SoftErrorChunkMarksSpan(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "soft_error_tool"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			if err := yield(toolsy.Chunk{
				Event:    toolsy.EventResult,
				Data:     []byte("budget exceeded"),
				MimeType: toolsy.MimeTypeText,
				IsError:  true,
			}); err != nil {
				return err
			}
			return nil
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(context.Background(), toolsy.NewRunEnv(nil), toolsy.ToolInput{}, func(toolsy.Chunk) error {
		return nil
	})
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.True(t, hasEvent(span, "tool.soft_error"))

	soft, ok := attrValue(span, "gen_ai.tool.soft_error")
	require.True(t, ok)
	assert.True(t, soft.AsBool())

	softText, ok := attrValue(span, "gen_ai.tool.soft_error_text")
	require.True(t, ok)
	assert.Equal(t, "budget exceeded", softText.AsString())
}

func TestMiddleware_ContentCapture_DisabledDefault_ErrorPath(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "capture_off_err"},
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return errors.New("sensitive failure")
		},
	}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"secret":true}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)

	span := rec.Ended()[0]
	assert.False(t, hasAttrKey(span, "langfuse.observation.input"))
	assert.False(t, hasAttrKey(span, "langfuse.observation.output"))
	assert.False(t, hasAttrKey(span, "gen_ai.tool.call.arguments"))
	assert.False(t, hasAttrKey(span, "gen_ai.tool.call.result"))
}

func TestMiddleware_ContentCapture_YieldErrorSkipsUndeliveredChunk(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "yield_err"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			if err := yield(toolsy.Chunk{Data: []byte("delivered"), MimeType: toolsy.MimeTypeText}); err != nil {
				return err
			}
			_ = yield(toolsy.Chunk{Data: []byte("undelivered"), MimeType: toolsy.MimeTypeText})
			return nil
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
	)(tool)

	require.NoError(t, wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error {
			if string(c.Data) == "undelivered" {
				return errors.New("consumer rejected")
			}
			return nil
		},
	))

	span := rec.Ended()[0]
	output, ok := attrValue(span, "langfuse.observation.output")
	require.True(t, ok)
	assert.Equal(t, "delivered", output.AsString())
	assert.NotContains(t, output.AsString(), "undelivered")
}

func TestMiddleware_ContentCapture_DisabledDefault(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{manifest: toolsy.ToolManifest{Name: "capture_off"}}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"q":"secret"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)

	span := rec.Ended()[0]
	assert.False(t, hasAttrKey(span, "langfuse.observation.input"))
	assert.False(t, hasAttrKey(span, "langfuse.observation.output"))
	assert.False(t, hasAttrKey(span, "gen_ai.tool.call.arguments"))
	assert.False(t, hasAttrKey(span, "gen_ai.tool.call.result"))
}

func TestMiddleware_ContentCapture_AlwaysSetsOperationAttrs(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tool := &stubTool{manifest: toolsy.ToolManifest{Name: "op_attrs"}}
	wrapped := WithTracing(WithTracerProvider(tp))(tool)

	require.NoError(t, wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{},
		func(toolsy.Chunk) error { return nil },
	))

	span := rec.Ended()[0]
	op, ok := attrValue(span, "gen_ai.operation.name")
	require.True(t, ok)
	assert.Equal(t, "execute_tool", op.AsString())

	obsType, ok := attrValue(span, "langfuse.observation.type")
	require.True(t, ok)
	assert.Equal(t, "tool", obsType.AsString())
}

func TestMiddleware_ContentCapture_Enabled(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	args := []byte(`{"city":"Berlin"}`)
	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "capture_on"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			if err := yield(toolsy.Chunk{Data: []byte("part-1"), MimeType: toolsy.MimeTypeText}); err != nil {
				return err
			}
			return yield(toolsy.Chunk{Data: []byte("part-2"), MimeType: toolsy.MimeTypeText})
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
	)(tool)

	require.NoError(t, wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: args},
		func(toolsy.Chunk) error { return nil },
	))

	span := rec.Ended()[0]
	input, ok := attrValue(span, "langfuse.observation.input")
	require.True(t, ok)
	assert.Equal(t, string(args), input.AsString())

	arguments, ok := attrValue(span, "gen_ai.tool.call.arguments")
	require.True(t, ok)
	assert.Equal(t, string(args), arguments.AsString())

	output, ok := attrValue(span, "langfuse.observation.output")
	require.True(t, ok)
	assert.Equal(t, "part-1part-2", output.AsString())

	result, ok := attrValue(span, "gen_ai.tool.call.result")
	require.True(t, ok)
	assert.Equal(t, "part-1part-2", result.AsString())
}

func TestMiddleware_ContentCapture_Truncation(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	hugeArgs := []byte(`{"payload":"` + strings.Repeat("x", 64) + `"}`)
	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "capture_trunc"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Data: []byte(strings.Repeat("y", 64)), MimeType: toolsy.MimeTypeText})
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
		WithMaxPayloadSize(10),
	)(tool)

	require.NoError(t, wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: hugeArgs},
		func(toolsy.Chunk) error { return nil },
	))

	span := rec.Ended()[0]
	limit := 10
	suffixLen := len("... [truncated]")

	input, ok := attrValue(span, "langfuse.observation.input")
	require.True(t, ok)
	assert.Contains(t, input.AsString(), "... [truncated]")
	assert.LessOrEqual(t, len(input.AsString()), limit+suffixLen)

	output, ok := attrValue(span, "langfuse.observation.output")
	require.True(t, ok)
	assert.Contains(t, output.AsString(), "... [truncated]")
	assert.LessOrEqual(t, len(output.AsString()), limit+suffixLen)
}

func TestMiddleware_ContentCapture_ErrorOutput(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	hugeErr := errors.New(strings.Repeat("e", 64))
	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "capture_err"},
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return hugeErr
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
		WithMaxPayloadSize(10),
	)(tool)

	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)

	span := rec.Ended()[0]
	output, ok := attrValue(span, "langfuse.observation.output")
	require.True(t, ok)
	assert.Contains(t, output.AsString(), "... [truncated]")
	assert.LessOrEqual(t, len(output.AsString()), 10+len("... [truncated]"))
	assert.False(t, hasAttrKey(span, "gen_ai.tool.call.result"))
}

func TestWithTracing_AsyncToolViaRegistry_SpanEndsAfterBackground(t *testing.T) {
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	block := make(chan struct{})
	var started atomic.Bool
	base := &stubTool{
		manifest: toolsy.ToolManifest{Name: "async_traced"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			started.Store(true)
			<-block
			return yield(toolsy.Chunk{
				Event:    toolsy.EventResult,
				Data:     []byte(`{"ok":true}`),
				MimeType: toolsy.MimeTypeJSON,
			})
		},
	}

	reg, err := toolsy.NewRegistryBuilder().
		Use(WithTracing(WithTracerProvider(tp))).
		Add(toolsy.AsAsyncTool(base)).
		Build()
	require.NoError(t, err)

	err = reg.Execute(
		context.Background(),
		toolsy.ToolCall{
			ToolName: "async_traced",
			Input:    toolsy.ToolInput{CallID: "async-1", ArgsJSON: []byte(`{}`)},
		},
		func(toolsy.Chunk) error { return nil },
	)
	require.NoError(t, err)
	assert.Empty(t, rec.Ended(), "tracing span must not end on sync AsyncAccepted path")

	require.Eventually(t, started.Load, time.Second, 10*time.Millisecond)
	close(block)
	require.Eventually(t, func() bool { return len(rec.Ended()) == 1 }, time.Second, 10*time.Millisecond)

	span := rec.Ended()[0]
	assert.Equal(t, "tool.execute.async_traced", span.Name())
}

func TestWithTracing_NestedAsyncTool_FailsRegistryBuild(t *testing.T) {
	tp, _ := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	base := &stubTool{
		manifest: toolsy.ToolManifest{Name: "nested_otel_base", Parameters: map[string]any{"type": "object"}},
	}
	nested := WithTracing(WithTracerProvider(tp))(toolsy.AsAsyncTool(toolsy.AsAsyncTool(base)))

	_, err := toolsy.NewRegistryBuilder().Add(nested).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple AsAsyncTool layers")
	assert.Contains(t, err.Error(), "nested_otel_base")
}
