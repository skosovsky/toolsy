package mail

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestDeclaredBodyRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      BodyRepresentation
		body, want string
		output     BodyRepresentation
		failure    error
	}{
		{"default_address", "", "  Contact <support@example.com> for details.\r\n", "  Contact <support@example.com> for details.\r\n", BodyPlainText, nil},
		{"plain_placeholder", BodyPlainText, "Use <branch>\n", "Use <branch>\n", BodyPlainText, nil},
		{"default_xml", "", "<root><value>1</value></root>", "<root><value>1</value></root>", BodyPlainText, nil},
		{"html_looking_plain", "", "<p>Hello</p>", "<p>Hello</p>", BodyPlainText, nil},
		{"empty_plain", "", "", "", BodyPlainText, nil},
		{"html", BodyHTML, "<p>Hello <strong>world</strong></p>", "Hello **world**", BodyMarkdown, nil},
		{"markdown_input", BodyMarkdown, "**bold**", "", "", ErrBodyRepresentation},
		{"mime_not_parsed", "text/html; charset=utf-8", "<p>Hello</p>", "", "", ErrBodyRepresentation},
		{"invalid_encoding", "", string([]byte{0xff}), "", "", ErrBodyEncoding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			tools, err := AsTools(nil, &mockReader{read: MessageBody{Body: tc.body, Representation: tc.input}})
			require.NoError(t, err)
			var chunks []toolsy.Chunk
			// Act.
			err = toolsy.WithErrorFormatter()(tools[1]).Execute(t.Context(), nil,
				toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"id"}`)},
				func(c toolsy.Chunk) error { chunks = append(chunks, c); return nil })
			// Assert.
			if tc.failure != nil {
				require.ErrorIs(t, err, tc.failure)
				require.Empty(t, chunks)
				var phase *toolsy.ResultContractError
				require.ErrorAs(t, err, &phase)
				classified, ok := toolsy.AsToolError(err)
				require.True(t, ok)
				require.Equal(t, toolsy.CodeInternal, classified.Code)
				require.False(t, classified.Retryable)
				return
			}
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			result := decodeMailChunk[readResult](t, chunks[0])
			require.Equal(t, "ID: id\nFrom: \nSubject: \nDate: \n\n"+tc.want, result.Body)
			require.Equal(t, tc.output, result.Representation)
		})
	}
}

func TestHTMLConversionFailureAndCancellationNeverFallback(t *testing.T) {
	cause := errors.New("converter failed")
	for _, tc := range []struct {
		name          string
		cancelDuring  bool
		invalidOutput bool
		want          error
	}{
		{"failure", false, false, cause},
		{"cancellation", true, false, context.Canceled},
		{"invalid_output", false, true, ErrBodyEncoding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			convert := func(string) (string, error) {
				calls++
				if tc.cancelDuring {
					cancel()
				}
				if tc.invalidOutput {
					return string([]byte{0xff}), nil
				}
				return "original HTML must not leak", cause
			}
			// Act.
			body, representation, err := convertHTMLBody(ctx, "<p>source</p>", convert)
			// Assert.
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, body)
			require.Empty(t, representation)
			require.Equal(t, 1, calls)
			if !errors.Is(tc.want, context.Canceled) {
				var phase *toolsy.ResultContractError
				require.ErrorAs(t, err, &phase)
			}
		})
	}
}

func TestDeclaredHTMLRawAndFinalBounds(t *testing.T) {
	// Arrange: declared HTML text expands through Markdown escaping, then JSON escaping adds bytes.
	msg := MessageBody{Body: `[x]`, Representation: BodyHTML}
	defaults := options{}
	applyDefaults(&defaults)
	result, err := doRead(t.Context(), &mockReader{read: msg}, readArgs{MessageID: "id"}, defaults)
	require.NoError(t, err)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		option   Option
		rejected bool
	}{
		{"body_exact", WithMaxBodyBytes(len(msg.Body)), false},
		{"body_over", WithMaxBodyBytes(len(msg.Body) - 1), true},
		{"source_exact", WithMaxSourceBytes(len(msg.Body) + len(msg.Representation)), false},
		{"source_over", WithMaxSourceBytes(len(msg.Body) + len(msg.Representation) - 1), true},
		{"item_exact", WithMaxItemBytes(len(msg.Body) + len(msg.Representation)), false},
		{"item_over", WithMaxItemBytes(len(msg.Body) + len(msg.Representation) - 1), true},
		{"wire_exact", WithMaxWireBytes(len(raw)), false},
		{"wire_over", WithMaxWireBytes(len(raw) - 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools, buildErr := AsTools(nil, &mockReader{read: msg}, tc.option)
			require.NoError(t, buildErr)
			yielded := 0
			// Act.
			err := tools[1].Execute(t.Context(), nil, toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"id"}`)},
				func(toolsy.Chunk) error { yielded++; return nil })
			// Assert.
			if tc.rejected {
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Zero(t, yielded)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, yielded)
			}
		})
	}
	// The final cap operates on converted text, not only the raw HTML body.
	require.Contains(t, result.Body, `\[x]`)
}
