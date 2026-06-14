package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

type mockSender struct {
	err error
}

func (m *mockSender) Send(ctx context.Context, msg OutgoingMessage) error {
	_ = ctx
	_ = msg
	return m.err
}

type mockReader struct {
	search []MessageSummary
	read   MessageBody
	err    error
}

func (m *mockReader) Search(ctx context.Context, query string, limit int) ([]MessageSummary, error) {
	_ = ctx
	_ = query
	_ = limit
	if m.err != nil {
		return nil, m.err
	}
	return m.search, nil
}

func (m *mockReader) Read(ctx context.Context, messageID string) (MessageBody, error) {
	_ = ctx
	_ = messageID
	if m.err != nil {
		return MessageBody{}, m.err
	}
	return m.read, nil
}

func decodeMailChunk[T any](t *testing.T, c toolsy.Chunk) T {
	t.Helper()
	require.Equal(t, toolsy.MimeTypeJSON, c.MimeType)
	var out T
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestMailSend_ArgsPassed(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil, WithReadOnly(false))
	require.NoError(t, err)
	require.Len(t, tools, 1)

	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"to":["a@b.com"],"subject":"Hi","body":"Hello"}`)},
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestMailSend_EmptyTo_ValidationToolError(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"to":[],"subject":"x","body":"y"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "recipient")
}

func TestMailSearchInbox_ReturnsMarkdown(t *testing.T) {
	reader := &mockReader{search: []MessageSummary{
		{ID: "1", From: "a@b.com", Subject: "Test", Date: "2026-03-11"},
	}}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	searchTool := tools[0]

	var result searchResult
	require.NoError(
		t,
		searchTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"query":"test","limit":5}`)},
			func(c toolsy.Chunk) error {
				result = decodeMailChunk[searchResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Results, "1")
	require.Contains(t, result.Results, "a@b.com")
	require.Contains(t, result.Results, "Test")
	require.Contains(t, result.Results, "|")
}

func TestMailSearch_EmptyQuery_ValidationToolError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"query":"   ","limit":5}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "query")
}

func TestMailReadMessage_ReturnsBody(t *testing.T) {
	reader := &mockReader{
		read: MessageBody{ID: "1", From: "x@y.com", Subject: "Subj", Body: "Body text", Date: "2026-03-11"},
	}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)
	readTool := tools[1]

	var result readResult
	require.NoError(
		t,
		readTool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"1"}`)},
			func(c toolsy.Chunk) error {
				result = decodeMailChunk[readResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Body, "Body text")
	require.Contains(t, result.Body, "x@y.com")
	require.Contains(t, result.Body, "Subj")
}

func TestMailRead_EmptyMessageID_ValidationToolError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "message_id")
}

func TestMailRead_WhitespaceOnlyMessageID_ValidationToolError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"   \t"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok)
	require.True(t, toolsy.ClientCorrectable(te.Code))
	assert.Equal(t, toolsy.CodeValidationFailed, te.Code)
	require.Contains(t, te.Reason, "message_id")
}

func TestMailRead_PlainTextWithAngleBrackets_NotConverted(t *testing.T) {
	plain := "Compare x < 5 and y > 0; use <- for assign."
	reader := &mockReader{read: MessageBody{ID: "1", From: "a@b.com", Subject: "Subj", Body: plain, Date: "2026-03-11"}}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	var result readResult
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"1"}`)},
			func(c toolsy.Chunk) error {
				result = decodeMailChunk[readResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Body, "x < 5")
	require.Contains(t, result.Body, "y > 0")
	require.Contains(t, result.Body, "<-")
}

func TestAsTools_ReadOnlyTrue_OnlyReadTools(t *testing.T) {
	sender := &mockSender{}
	reader := &mockReader{}
	tools, err := AsTools(sender, reader, WithReadOnly(true))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "mail_search_inbox", tools[0].Manifest().Name)
	require.Equal(t, "mail_read_message", tools[1].Manifest().Name)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().ReadOnly)
}

func TestAsTools_NilSender_OnlyReadTools(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "mail_search_inbox", tools[0].Manifest().Name)
	require.Equal(t, "mail_read_message", tools[1].Manifest().Name)
	require.True(t, tools[0].Manifest().ReadOnly)
	require.True(t, tools[1].Manifest().ReadOnly)
}

func TestAsTools_NilReader_OnlySendTool(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "mail_send", tools[0].Manifest().Name)
	require.True(t, tools[0].Manifest().Dangerous)
	require.True(t, tools[0].Manifest().RequiresConfirmation)
}

func TestAsTools_BothNil_Error(t *testing.T) {
	_, err := AsTools(nil, nil)
	require.Error(t, err)
}

func TestMailSend_HandlerError_Wrapped(t *testing.T) {
	sender := &mockSender{err: errors.New("smtp failed")}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.NewRunEnv(nil),
		toolsy.ToolInput{ArgsJSON: []byte(`{"to":["a@b.com"],"subject":"x","body":"y"}`)},
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	te, ok := toolsy.AsToolError(err)
	require.True(t, ok, "expected ToolError, got %v", err)
	require.Equal(t, toolsy.CodeInternal, te.Code)
	require.Error(t, te.Unwrap())
}

func TestMailRead_HTMLBody_NormalizedToMarkdown(t *testing.T) {
	reader := &mockReader{read: MessageBody{
		ID: "1", From: "a@b.com", Subject: "Subj", Date: "2026-03-11",
		Body: "<p>Hello <strong>world</strong></p>",
	}}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	var result readResult
	require.NoError(
		t,
		tools[1].Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"1"}`)},
			func(c toolsy.Chunk) error {
				result = decodeMailChunk[readResult](t, c)
				return nil
			},
		),
	)
	require.Contains(t, result.Body, "Hello")
	require.Contains(t, result.Body, "world")
	require.NotContains(t, result.Body, "<p>")
	require.NotContains(t, result.Body, "<strong>")
}

func TestNormalizeBody_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	html := "<p>" + strings.Repeat("x", 200) + "</p>"
	got := normalizeBody(ctx, html)
	require.Equal(t, html, got)
}
