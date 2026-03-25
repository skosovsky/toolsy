package mail

import (
	"context"
	"errors"
	"strings"
	"testing"

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

func TestMailSend_ArgsPassed(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil, WithReadOnly(false))
	require.NoError(t, err)
	require.Len(t, tools, 1)

	require.NoError(
		t,
		tools[0].Execute(
			context.Background(),
			toolsy.RunContext{},
			[]byte(`{"to":["a@b.com"],"subject":"Hi","body":"Hello"}`),
			func(toolsy.Chunk) error { return nil },
		),
	)
}

func TestMailSend_EmptyTo_ClientError(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{"to":[],"subject":"x","body":"y"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "recipient")
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
			toolsy.RunContext{},
			[]byte(`{"query":"test","limit":5}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(searchResult); ok {
						result = r
					}
				}
				return nil
			},
		),
	)
	require.Contains(t, result.Results, "1")
	require.Contains(t, result.Results, "a@b.com")
	require.Contains(t, result.Results, "Test")
	require.Contains(t, result.Results, "|")
}

func TestMailSearch_EmptyQuery_ClientError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{"query":"   ","limit":5}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "query")
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
			toolsy.RunContext{},
			[]byte(`{"message_id":"1"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(readResult); ok {
						result = r
					}
				}
				return nil
			},
		),
	)
	require.Contains(t, result.Body, "Body text")
	require.Contains(t, result.Body, "x@y.com")
	require.Contains(t, result.Body, "Subj")
}

func TestMailRead_EmptyMessageID_ClientError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "message_id")
}

func TestMailRead_WhitespaceOnlyMessageID_ClientError(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)

	err = tools[1].Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{"message_id":"   \t"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	require.True(t, toolsy.IsClientError(err))
	require.Contains(t, err.Error(), "message_id")
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
			toolsy.RunContext{},
			[]byte(`{"message_id":"1"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(readResult); ok {
						result = r
					}
				}
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
	require.Equal(t, "mail_search_inbox", tools[0].Name())
	require.Equal(t, "mail_read_message", tools[1].Name())
}

func TestAsTools_NilSender_OnlyReadTools(t *testing.T) {
	reader := &mockReader{}
	tools, err := AsTools(nil, reader)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "mail_search_inbox", tools[0].Name())
	require.Equal(t, "mail_read_message", tools[1].Name())
}

func TestAsTools_NilReader_OnlySendTool(t *testing.T) {
	sender := &mockSender{}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "mail_send", tools[0].Name())
}

func TestAsTools_BothNil_Error(t *testing.T) {
	_, err := AsTools(nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one")
}

func TestMailSend_HandlerError_Wrapped(t *testing.T) {
	sender := &mockSender{err: errors.New("smtp failed")}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)

	err = tools[0].Execute(
		context.Background(),
		toolsy.RunContext{},
		[]byte(`{"to":["a@b.com"],"subject":"x","body":"y"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	msg := err.Error()
	require.True(
		t,
		strings.Contains(msg, "toolkit/mail") || strings.Contains(msg, "smtp") ||
			strings.Contains(msg, "internal system error"),
		"error should mention toolkit/mail, smtp, or system: %s",
		msg,
	)
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
			toolsy.RunContext{},
			[]byte(`{"message_id":"1"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(readResult); ok {
						result = r
					}
				}
				return nil
			},
		),
	)
	require.Contains(t, result.Body, "Hello")
	require.Contains(t, result.Body, "world")
	require.NotContains(t, result.Body, "<p>")
	require.NotContains(t, result.Body, "<strong>")
}
