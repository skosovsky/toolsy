package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/skosovsky/toolsy"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

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
	if o.maxBodyBytes < 0 || o.maxSourceBytes < 0 || o.maxItemBytes < 0 || o.maxSearchResults < 0 ||
		o.maxWireBytes < 0 {
		return nil, errors.New("toolkit/mail: limits must be nonnegative")
	}
	applyDefaults(&o)

	var tools []toolsy.Tool
	if sender != nil && !o.readOnly {
		t, err := toolsy.NewTool[sendArgs, sendResult](
			o.sendName,
			o.sendDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args sendArgs) (sendResult, error) {
				return doSend(ctx, sender, args, o)
			},
			toolsy.WithDangerous(),
			toolsy.WithRequiresConfirmation(),
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
			func(ctx context.Context, _ *toolsy.RunEnv, args searchArgs) (searchResult, error) {
				return doSearch(ctx, reader, args, o)
			},
			toolsy.WithReadOnly(),
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/mail: build search tool: %w", err)
		}
		tools = append(tools, searchTool)

		readTool, err := toolsy.NewTool[readArgs, readResult](
			o.readName,
			o.readDesc,
			func(ctx context.Context, _ *toolsy.RunEnv, args readArgs) (readResult, error) {
				return doRead(ctx, reader, args, o)
			},
			toolsy.WithReadOnly(),
		)
		if err != nil {
			return nil, fmt.Errorf("toolkit/mail: build read tool: %w", err)
		}
		tools = append(tools, readTool)
	}
	return tools, nil
}

func doSend(ctx context.Context, sender MailSender, args sendArgs, o options) (sendResult, error) {
	if len(args.To) == 0 {
		return sendResult{}, toolsy.NewValidationError("at least one recipient (to) is required")
	}
	if len(args.Body) > o.maxBodyBytes {
		return sendResult{}, toolsy.NewValidationError("mail body exceeds configured byte limit")
	}
	result := sendResult{Status: "sent"}
	if err := checkWire(result, o.maxWireBytes); err != nil {
		return sendResult{}, err
	}
	err := sender.Send(ctx, OutgoingMessage(args))
	if err != nil {
		return sendResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/mail: send: %w", err))
	}
	return result, nil
}

func doSearch(ctx context.Context, reader MailReader, args searchArgs, o options) (searchResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return searchResult{}, toolsy.NewValidationError("query is required (empty query would return entire inbox)")
	}
	limit := args.Limit
	if limit < 0 || limit > o.maxSearchResults {
		return searchResult{}, toolsy.NewValidationError("search limit exceeds configured count bounds")
	}
	if limit == 0 {
		limit = min(defaultSearchLimit, o.maxSearchResults)
	}
	list, err := reader.Search(ctx, query, limit)
	if err != nil {
		return searchResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/mail: search: %w", err))
	}
	if len(list) > limit {
		return searchResult{}, limitError("provider search count")
	}
	remaining := o.maxSourceBytes
	for _, m := range list {
		if err := checkFields(o.maxItemBytes, &remaining, m.ID, m.From, m.Subject, m.Date); err != nil {
			return searchResult{}, err
		}
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
	result := searchResult{Results: b.String()}
	if err := checkWire(result, o.maxWireBytes); err != nil {
		return searchResult{}, err
	}
	return result, nil
}

func doRead(ctx context.Context, reader MailReader, args readArgs, o options) (readResult, error) {
	messageID := strings.TrimSpace(args.MessageID)
	if messageID == "" {
		return readResult{}, toolsy.NewValidationError("message_id is required")
	}
	msg, err := reader.Read(ctx, messageID)
	if err != nil {
		return readResult{}, toolsy.NewInternalError(fmt.Errorf("toolkit/mail: read: %w", err))
	}
	remaining := o.maxSourceBytes
	if len(msg.Body) > o.maxBodyBytes {
		return readResult{}, limitError("provider body bytes")
	}
	if err := checkFields(o.maxItemBytes, &remaining, msg.ID, msg.From, msg.Subject, msg.Body, msg.Date); err != nil {
		return readResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return readResult{}, err
	}
	body := normalizeBody(ctx, msg.Body)
	if err := ctx.Err(); err != nil {
		return readResult{}, err
	}
	var b strings.Builder
	b.WriteString("ID: ")
	b.WriteString(messageID)
	if msg.ID != "" && msg.ID != messageID {
		b.WriteString("\nProvider ID: ")
		b.WriteString(msg.ID)
	}
	b.WriteString("\nFrom: ")
	b.WriteString(msg.From)
	b.WriteString("\nSubject: ")
	b.WriteString(msg.Subject)
	b.WriteString("\nDate: ")
	b.WriteString(msg.Date)
	b.WriteString("\n\n")
	b.WriteString(body)
	result := readResult{Body: b.String()}
	if err := checkWire(result, o.maxWireBytes); err != nil {
		return readResult{}, err
	}
	return result, nil
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
// HTML conversion is best-effort cancellable via ctx.
func normalizeBody(ctx context.Context, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if !looksLikeHTML(body) {
		return body
	}
	if err := ctx.Err(); err != nil {
		return body
	}
	md, err := htmltomarkdown.ConvertString(body)
	if err != nil {
		return body
	}
	if err := ctx.Err(); err != nil {
		return body
	}
	return strings.TrimSpace(md)
}

func limitError(bound string) error {
	return toolsy.NewValidationError(
		"mail result exceeds configured limit: " + bound + "; reader does not support continuation",
	)
}

// Subtraction keeps aggregate checks safe even for hostile provider lengths.
func checkFields(itemLimit int, remaining *int, fields ...string) error {
	itemRemaining := itemLimit
	for _, field := range fields {
		if len(field) > itemRemaining {
			return limitError("item bytes")
		}
		if len(field) > *remaining {
			return limitError("source bytes")
		}
		itemRemaining -= len(field)
		*remaining -= len(field)
	}
	return nil
}

func checkWire(result any, maxBytes int) error {
	data, err := json.Marshal(result)
	if err != nil {
		return toolsy.NewInternalError(err)
	}
	if len(data) > maxBytes {
		return limitError("encoded JSON bytes")
	}
	return nil
}
