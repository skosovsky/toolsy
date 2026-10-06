package toolsyotel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestD25DefaultHasNoVendorAttributes(t *testing.T) {
	for _, capture := range []bool{false, true} {
		// Arrange: default mapping must not depend on a selected telemetry backend.
		tp, rec := newSpanRecorder()
		tool := &stubTool{manifest: toolsy.ToolManifest{Name: "portable"}}
		wrapped := WithTracing(WithTracerProvider(tp), WithContentCapture(capture))(tool)
		// Act.
		require.NoError(
			t,
			wrapped.Execute(
				t.Context(),
				toolsy.NewRunEnv(nil),
				toolsy.ToolInput{CallID: "call", ArgsJSON: []byte(`{}`)},
				func(toolsy.Chunk) error { return nil },
			),
		)
		// Assert: content opt-in does not implicitly opt into a vendor mapping.
		require.Len(t, rec.Ended(), 1)
		for _, attr := range rec.Ended()[0].Attributes() {
			require.False(t, strings.HasPrefix(string(attr.Key), "langfuse."), "unexpected vendor attr %s", attr.Key)
		}
		require.NoError(t, tp.Shutdown(context.Background()))
	}
}

func TestVendorMappingContentMatrix(t *testing.T) {
	const marker = "CONTENT_MARKER"
	for _, path := range []string{"result", "soft", "hard", "panic"} {
		for _, vendor := range []bool{false, true} {
			for _, capture := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/vendor=%t/capture=%t", path, vendor, capture), func(t *testing.T) {
					checkVendorMappingPath(t, path, marker, vendor, capture)
				})
			}
		}
	}
}

func TestChunkRedactionBoundaryIsNotWholeSecretSanitation(t *testing.T) {
	const secret = "SECRET_TOKEN"
	for _, omitWholeChunk := range []bool{false, true} {
		for _, vendor := range []bool{false, true} {
			t.Run(fmt.Sprintf("whole=%t/vendor=%t", omitWholeChunk, vendor), func(t *testing.T) {
				checkChunkRedactionBoundary(t, secret, omitWholeChunk, vendor)
			})
		}
	}
}

func checkVendorMappingPath(t *testing.T, path, marker string, vendor, capture bool) {
	t.Helper()
	// Arrange: vendor selection and sensitive content selection are independent.
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	options := []Option{WithTracerProvider(tp), WithLangfuseCompatibility(vendor), WithContentCapture(capture)}
	// Act.
	executeContentPolicyPath(t, path, marker, contentPolicyTool(path, marker), options)
	// Assert: every observable location obeys capture, and result never describes failure.
	require.Len(t, rec.Ended(), 1)
	span := rec.Ended()[0]
	text := spanContent(span)
	require.Equal(t, vendor, hasAttrKey(span, "langfuse.observation.type"))
	if capture {
		require.Contains(t, text, marker)
	} else {
		require.NotContains(t, text, marker)
	}
	require.Equal(t, vendor && capture, hasAttrKey(span, "langfuse.observation.input"))
	require.Equal(t, vendor && capture && path != "panic", hasAttrKey(span, "langfuse.observation.output"))
	require.Equal(t, capture && path == "result", hasAttrKey(span, "gen_ai.tool.call.result"))
	if vendor && capture && path != "panic" {
		portableKey := "toolsy.tool.output"
		if path == "hard" {
			portableKey = "toolsy.tool.error"
		}
		portable, ok := attrValue(span, portableKey)
		require.True(t, ok)
		mapped, ok := attrValue(span, "langfuse.observation.output")
		require.True(t, ok)
		require.Equal(t, portable.AsString(), mapped.AsString())
	}
}

func checkChunkRedactionBoundary(t *testing.T, secret string, omitWholeChunk, vendor bool) {
	t.Helper()
	// Arrange: a secret exists only after independently redacted chunks are concatenated.
	tp, rec := newSpanRecorder()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	var outputCalls int
	tool := &stubTool{
		manifest: toolsy.ToolManifest{Name: "boundary"},
		execute: func(_ context.Context, _ *toolsy.RunEnv, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
			for _, part := range []string{"SECRET_", "TOKEN"} {
				if err := yield(toolsy.Chunk{Data: []byte(part), MimeType: toolsy.MimeTypeText}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	wrapped := WithTracing(
		WithTracerProvider(tp),
		WithContentCapture(true),
		WithLangfuseCompatibility(vendor),
		WithContentRedactor(func(kind ContentKind, text string) string {
			if kind == ContentOutput {
				outputCalls++
				if omitWholeChunk {
					return "[omitted]"
				}
			}
			return strings.ReplaceAll(text, secret, "[redacted]")
		}),
	)(
		tool,
	)
	// Act.
	require.NoError(
		t,
		wrapped.Execute(
			t.Context(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
	// Assert: matching whole secrets per chunk is insufficient; host may omit classified chunks.
	require.Equal(t, 2, outputCalls)
	output, ok := attrValue(rec.Ended()[0], "toolsy.tool.output")
	require.True(t, ok)
	if omitWholeChunk {
		require.NotContains(t, spanContent(rec.Ended()[0]), secret)
	} else {
		require.Equal(t, secret, output.AsString())
	}
	if vendor {
		mapped, exists := attrValue(rec.Ended()[0], "langfuse.observation.output")
		require.True(t, exists)
		require.Equal(t, output.AsString(), mapped.AsString())
	}
}

func TestVendorMappingInvalidUTF8Redactor(t *testing.T) {
	for _, vendor := range []bool{false, true} {
		for _, path := range []string{"result", "soft", "hard", "panic"} {
			t.Run(fmt.Sprintf("%s/vendor=%t", path, vendor), func(t *testing.T) {
				// Arrange: host redactors can return invalid Go strings, including below the cap.
				tp, rec := newSpanRecorder()
				t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
				opts := []Option{
					WithTracerProvider(tp),
					WithContentCapture(true),
					WithLangfuseCompatibility(vendor),
					WithMaxPayloadSize(3),
					WithContentRedactor(func(ContentKind, string) string { return string([]byte{0xff}) }),
				}
				// Act.
				executeContentPolicyPath(t, path, "original", contentPolicyTool(path, "original"), opts)
				// Assert: normalization does not expand any exported payload beyond its byte budget.
				require.Len(t, rec.Ended(), 1)
				assertCapturedByteCap(t, rec.Ended()[0], 3)
				args, ok := attrValue(rec.Ended()[0], "gen_ai.tool.call.arguments")
				require.True(t, ok)
				require.Equal(t, "\uFFFD", args.AsString())
			})
		}
	}
}
