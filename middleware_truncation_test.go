package toolsy

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithTruncation_TruncatesTextPayload(t *testing.T) {
	inner := newMiddlewareMinTool(
		"truncate",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte("abcdefghijklmnopqrstuvwxyz"),
				MimeType: MimeTypeText,
				Metadata: map[string]any{"k": "v"},
			})
		},
	)

	wrapped := WithTruncation(12, WithTruncationSuffix("..."))(inner)

	var got Chunk
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		got = c
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, EventResult, got.Event)
	assert.Equal(t, MimeTypeText, got.MimeType)
	assert.Equal(t, "v", got.Metadata["k"])
	assert.Equal(t, "abcdefghi...", string(got.Data))
	assert.Equal(t, 12, utf8.RuneCount(got.Data))
}

func TestWithTruncation_TruncatesMarkdownPayload(t *testing.T) {
	input := "# Heading\nSome **very long** markdown text"
	inner := newMiddlewareMinTool(
		"markdown",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte(input),
				MimeType: "text/markdown; charset=utf-8",
				Metadata: map[string]any{"kind": "md"},
			})
		},
	)

	wrapped := WithTruncation(14, WithTruncationSuffix("..."))(inner)

	var got Chunk
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		got = c
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, EventResult, got.Event)
	assert.Equal(t, "text/markdown; charset=utf-8", got.MimeType)
	assert.Equal(t, "md", got.Metadata["kind"])
	assert.NotEqual(t, input, string(got.Data))
	assert.True(t, strings.HasSuffix(string(got.Data), "..."))
	assert.Equal(t, 14, utf8.RuneCount(got.Data))
	assert.True(t, utf8.Valid(got.Data))
}

func TestWithTruncation_DoesNotTruncateBinaryPayload(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
	inner := newMiddlewareMinTool(
		"binary",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     raw,
				MimeType: MimeTypePNG,
			})
		},
	)

	wrapped := WithTruncation(2, WithTruncationSuffix("..."))(inner)

	var got Chunk
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		got = c
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, raw, got.Data)
}

func TestWithTruncation_JSONIsOptIn(t *testing.T) {
	jsonText := `{"value":"abcdefghijklmnopqrstuvwxyz"}`
	inner := newMiddlewareMinTool(
		"json",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte(jsonText),
				MimeType: MimeTypeJSON,
			})
		},
	)

	withoutJSON := WithTruncation(10, WithTruncationSuffix("..."))(inner)
	withJSON := WithTruncation(10, WithTruncationSuffix("..."), WithTruncationIncludeJSON(true))(inner)

	var gotWithout, gotWith Chunk
	err := withoutJSON.Execute(
		context.Background(),
		RunContext{},
		ToolInput{ArgsJSON: []byte(`{}`)},
		func(c Chunk) error {
			gotWithout = c
			return nil
		},
	)
	require.NoError(t, err)
	err = withJSON.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		gotWith = c
		return nil
	})
	require.NoError(t, err)

	assert.JSONEq(t, jsonText, string(gotWithout.Data))
	assert.NotEqual(t, jsonText, string(gotWith.Data))
	assert.True(t, strings.HasSuffix(string(gotWith.Data), "..."))
}

func TestWithTruncation_PreservesUTF8Boundaries(t *testing.T) {
	input := "приветмир"
	inner := newMiddlewareMinTool(
		"utf8",
		func(_ context.Context, _ RunContext, _ ToolInput, yield func(Chunk) error) error {
			return yield(Chunk{
				Event:    EventResult,
				Data:     []byte(input),
				MimeType: MimeTypeText,
			})
		},
	)

	wrapped := WithTruncation(6, WithTruncationSuffix("..."))(inner)

	var got string
	err := wrapped.Execute(context.Background(), RunContext{}, ToolInput{ArgsJSON: []byte(`{}`)}, func(c Chunk) error {
		got = string(c.Data)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(got))
	assert.Equal(t, 6, utf8.RuneCountInString(got))
	assert.Equal(t, "при...", got)
}
