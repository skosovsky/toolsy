package toolsyotel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/history"
)

// Include every observable span location, particularly exception event attributes
// and status descriptions, rather than checking only the primary output attribute.
func spanContent(span sdktrace.ReadOnlySpan) string {
	var b strings.Builder
	fmt.Fprint(&b, span.Name(), span.Status().Description, span.Attributes())
	for _, event := range span.Events() {
		fmt.Fprint(&b, event.Name, event.Attributes)
	}
	return b.String()
}

func TestContentPolicy_AllPayloadPaths(t *testing.T) {
	const secret = "SECRET_MARKER_938"
	for _, path := range []string{"result", "soft", "hard", "panic", "semantic"} {
		for _, capture := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/capture=%t", path, capture), func(t *testing.T) {
				// Arrange.
				tp, rec := newSpanRecorder()
				t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
				opts := []Option{WithTracerProvider(tp), WithContentCapture(capture)}
				tool := contentPolicyTool(path, secret)
				// Act.
				executeContentPolicyPath(t, path, secret, tool, opts)
				// Assert.
				require.Len(t, rec.Ended(), 1)
				text := spanContent(rec.Ended()[0])
				if capture {
					require.Contains(t, text, secret)
				} else {
					require.NotContains(t, text, secret)
				}
			})
		}
	}
}

func TestContentPolicy_RedactionPrecedesExactCap(t *testing.T) {
	const secret = "SECRET_MARKER_AFTER_LONG_PREFIX"
	for _, path := range []string{"result", "soft", "hard", "panic", "semantic"} {
		t.Run(path, func(t *testing.T) {
			// Arrange: matching the end of a long input proves redaction sees uncapped content.
			tp, rec := newSpanRecorder()
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
			raw := strings.Repeat("界", 100) + secret
			opts := []Option{
				WithTracerProvider(tp),
				WithContentCapture(true),
				WithMaxPayloadSize(7),
				WithContentRedactor(func(_ ContentKind, text string) string {
					if strings.Contains(text, secret) {
						return "SAFE"
					}
					return text
				}),
			}
			tool := contentPolicyTool(path, raw)
			// Act.
			executeContentPolicyPath(t, path, raw, tool, opts)
			// Assert.
			require.Len(t, rec.Ended(), 1)
			span := rec.Ended()[0]
			require.Contains(t, spanContent(span), "SAFE")
			require.NotContains(t, spanContent(span), secret)
			assertCapturedByteCap(t, span, 7)
		})
	}
}

func TestContentPolicy_RedactorPanicFailsClosed(t *testing.T) {
	// Arrange.
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "redactor_failure"},
		execute: func(context.Context, *toolsy.RunEnv, toolsy.ToolInput, func(toolsy.Chunk) error) error {
			return errors.New("SECRET_MARKER")
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
		WithMaxPayloadSize(8),
		WithContentRedactor(func(ContentKind, string) string { panic("SECRET_MARKER") }),
	)(
		tool,
	)
	// Act.
	err := wrapped.Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte("SECRET_MARKER")},
		func(toolsy.Chunk) error { return nil },
	)
	// Assert.
	require.EqualError(t, err, "SECRET_MARKER")
	require.Len(t, rec.Ended(), 1)
	require.NotContains(t, spanContent(rec.Ended()[0]), "SECRET_MARKER")
}

func contentPolicyTool(path, content string) *stubTool {
	return &stubTool{
		manifest: toolsy.ToolManifest{Name: "policy"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			switch path {
			case "hard":
				return errors.New(content)
			case "panic":
				panic(content)
			default:
				return yield(
					toolsy.Chunk{IsError: path == "soft", Data: []byte(content), MimeType: toolsy.MimeTypeText},
				)
			}
		},
	}
}

func executeContentPolicyPath(t *testing.T, path, content string, tool *stubTool, opts []Option) {
	t.Helper()
	if path == "semantic" {
		RecordSemanticTruncation(context.Background(), history.SemanticTruncationReport{}, errors.New(content), opts...)
		return
	}
	if path == "panic" {
		defer func() { require.Equal(t, content, recover()) }()
	}
	_ = WithTracing(
		opts...)(
		tool,
	).Execute(context.Background(), toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: []byte(content)}, func(toolsy.Chunk) error { return nil })
}

func assertCapturedByteCap(t *testing.T, span sdktrace.ReadOnlySpan, limit int) {
	t.Helper()
	check := func(key, value string) {
		if strings.Contains(key, "input") || strings.Contains(key, "output") || strings.Contains(key, "arguments") ||
			strings.Contains(key, "result") ||
			strings.Contains(key, "text") ||
			strings.HasPrefix(key, "exception.") {
			require.LessOrEqual(t, len(value), limit)
			require.True(t, utf8.ValidString(value))
		}
	}
	for _, attr := range span.Attributes() {
		check(string(attr.Key), attr.Value.AsString())
	}
	for _, ev := range span.Events() {
		for _, attr := range ev.Attributes {
			check(string(attr.Key), attr.Value.AsString())
		}
	}
}
