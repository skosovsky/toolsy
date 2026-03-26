package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/internal/textutil"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// maxSearchInboxLimit is the maximum number of inbox search results per tool call.
const maxSearchInboxLimit = 100

// OutgoingMessage is passed to MailSender.Send.
type OutgoingMessage struct {
	To      []string
	Subject string
	Body    string
}

// MailSender is implemented by the orchestrator (SMTP, SendGrid, Resend, etc.).
//
//nolint:revive // name matches toolskit spec
type MailSender interface {
	Send(ctx context.Context, msg OutgoingMessage) error
}

// MessageSummary is a row from inbox search.
type MessageSummary struct {
	ID      string
	From    string
	Subject string
	Date    string
}

// MessageBody is the full message content.
type MessageBody struct {
	ID      string
	From    string
	Subject string
	Body    string
	Date    string
}

// MailReader is implemented by the orchestrator (IMAP, Gmail API, etc.).
//
//nolint:revive // name matches toolskit spec
type MailReader interface {
	Search(ctx context.Context, query string, limit int) ([]MessageSummary, error)
	Read(ctx context.Context, messageID string) (MessageBody, error)
}

type sendArgs struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

type sendResult struct {
	Status string `json:"status"`
}

type searchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type searchResult struct {
	Results string `json:"results"`
}

type readArgs struct {
	MessageID string `json:"message_id"`
}

type readResult struct {
	Body string `json:"body"`
}

// AsTools returns mail_send (if sender != nil and not readOnly), mail_search_inbox and mail_read_message (if reader != nil).
// At least one of sender or reader must be non-nil.
func AsTools(sender MailSender, reader MailReader, opts ...Option) ([]toolsy.Tool, error) {
	if sender == nil && reader == nil {
		return nil, errors.New("toolkit/mail: at least one of sender or reader must be provided")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	applyDefaults(&o)

	var tools []toolsy.Tool
	if sender != nil && !o.readOnly {
		t, err := toolsy.NewTool[sendArgs, sendResult](
			o.sendName,
			o.sendDesc,
			func(ctx context.Context, _ toolsy.RunContext, args sendArgs) (sendResult, error) {
				return doSend(ctx, sender, args, o.maxBodyBytes)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/mail: build send tool: %w", err)
		}
		tools = append(tools, t)
	}
	if reader != nil {
		searchTool, err := toolsy.NewTool[searchArgs, searchResult](
			o.searchName,
			o.searchDesc,
			func(ctx context.Context, _ toolsy.RunContext, args searchArgs) (searchResult, error) {
				return doSearch(ctx, reader, args, o.maxBodyBytes)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/mail: build search tool: %w", err)
		}
		tools = append(tools, searchTool)

		readTool, err := toolsy.NewTool[readArgs, readResult](
			o.readName,
			o.readDesc,
			func(ctx context.Context, _ toolsy.RunContext, args readArgs) (readResult, error) {
				return doRead(ctx, reader, args, o.maxBodyBytes)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/mail: build read tool: %w", err)
		}
		tools = append(tools, readTool)
	}
	return tools, nil
}

func doSend(ctx context.Context, sender MailSender, args sendArgs, maxBodyBytes int) (sendResult, error) {
	if len(args.To) == 0 {
		return sendResult{}, &toolsy.ClientError{
			Reason:    "at least one recipient (to) is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	body := args.Body
	if maxBodyBytes > 0 && len(body) > maxBodyBytes {
		body = textutil.TruncateStringUTF8(body, maxBodyBytes, "\n[Truncated]")
	}
	err := sender.Send(ctx, OutgoingMessage{To: args.To, Subject: args.Subject, Body: body})
	if err != nil {
		return sendResult{}, fmt.Errorf("toolkit/mail: send: %w", err)
	}
	return sendResult{Status: "sent"}, nil
}

func doSearch(ctx context.Context, reader MailReader, args searchArgs, _ int) (searchResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return searchResult{}, &toolsy.ClientError{
			Reason:    "query is required (empty query would return entire inbox)",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > maxSearchInboxLimit {
		limit = maxSearchInboxLimit
	}
	list, err := reader.Search(ctx, query, limit)
	if err != nil {
		return searchResult{}, fmt.Errorf("toolkit/mail: search: %w", err)
	}
	var b strings.Builder
	b.WriteString("| ID | From | Subject | Date |\n|----|------|---------|------|\n")
	for _, m := range list {
		b.WriteString("| ")
		b.WriteString(escapeCell(m.ID))
		b.WriteString(" | ")
		b.WriteString(escapeCell(m.From))
		b.WriteString(" | ")
		b.WriteString(escapeCell(m.Subject))
		b.WriteString(" | ")
		b.WriteString(escapeCell(m.Date))
		b.WriteString(" |\n")
	}
	return searchResult{Results: b.String()}, nil
}

func doRead(ctx context.Context, reader MailReader, args readArgs, maxBodyBytes int) (readResult, error) {
	messageID := strings.TrimSpace(args.MessageID)
	if messageID == "" {
		return readResult{}, &toolsy.ClientError{
			Reason:    "message_id is required",
			Retryable: false,
			Err:       toolsy.ErrValidation,
		}
	}
	msg, err := reader.Read(ctx, messageID)
	if err != nil {
		return readResult{}, fmt.Errorf("toolkit/mail: read: %w", err)
	}
	body := normalizeBody(msg.Body)
	if maxBodyBytes > 0 && len(body) > maxBodyBytes {
		body = textutil.TruncateStringUTF8(body, maxBodyBytes, "\n[Truncated]")
	}
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(msg.From)
	b.WriteString("\nSubject: ")
	b.WriteString(msg.Subject)
	b.WriteString("\nDate: ")
	b.WriteString(msg.Date)
	b.WriteString("\n\n")
	b.WriteString(body)
	return readResult{Body: b.String()}, nil
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// looksLikeHTML returns true if body contains what appears to be an HTML tag (e.g. <p>, </div>),
// so plain text with angle brackets (e.g. "x < 5" or XML snippets) is not converted.
func looksLikeHTML(body string) bool {
	for i := range len(body) {
		if body[i] != '<' {
			continue
		}
		j := i + 1
		for j < len(body) && (body[j] == ' ' || body[j] == '/') {
			j++
		}
		if j < len(body) && (body[j] >= 'a' && body[j] <= 'z' || body[j] >= 'A' && body[j] <= 'Z') {
			return true
		}
	}
	return false
}

// normalizeBody converts HTML body to readable Markdown/text so the agent does not see raw tags.
// Only runs conversion when body looks like HTML (contains tag-like patterns); plain text with < is left as-is.
func normalizeBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if !looksLikeHTML(body) {
		return body
	}
	md, err := htmltomarkdown.ConvertString(body)
	if err != nil {
		return body
	}
	return strings.TrimSpace(md)
}
