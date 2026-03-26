# Toolsy: Mail Toolkit (mail)

**Description:** Lets the agent send email and read/search inbox via MailSender and MailReader interfaces. Implementations can use SMTP, IMAP, SendGrid, Resend, Gmail API, etc.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/mail
```

**Dependencies:** `github.com/skosovsky/toolsy`, `github.com/JohannesKaufmann/html-to-markdown/v2` (for HTML body normalization).

## Available tools

| Tool                 | Description                    | Input                                    |
|----------------------|--------------------------------|------------------------------------------|
| `mail_send`          | Send an email                  | `{"to": ["string"], "subject": "string", "body": "string"}` |
| `mail_search_inbox`  | Search inbox by query          | `{"query": "string", "limit": int}`      |
| `mail_read_message`  | Read a single message by ID    | `{"message_id": "string"}`              |

Tools are generated only when the corresponding interface is provided: nil sender skips mail_send; nil reader skips both search and read. At least one must be non-nil. Body in `mail_read_message` is converted to Markdown only when it looks like HTML (tag-like patterns); plain text with `<` (e.g. "x < 5") is left as-is. `message_id` is trimmed; whitespace-only is rejected with ClientError.

## Configuration & Security

> **Warning:** Credentials and network access are the responsibility of your MailSender/MailReader implementations. Use read-only or nil sender in production if the agent must not send mail.

- **Nil-safe:** Pass `nil` for sender to get only search/read tools; pass `nil` for reader to get only send. At least one must be non-nil.
- **WithReadOnly(true):** Disables mail_send even when sender is non-nil.
- **WithMaxBodyBytes(n):** Limits body size for send and read (default 256KB).
- **Query required:** Empty or whitespace-only `query` in mail_search_inbox returns ClientError (avoids dumping entire inbox). Same for `message_id` in mail_read_message.

## Quick start

```go
package main

import (
	"context"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/mail"
)

// Minimal in-memory implementations for a compilable example.
type noopSender struct{}
func (noopSender) Send(context.Context, mail.OutgoingMessage) error { return nil }

type noopReader struct{}
func (noopReader) Search(context.Context, string, int) ([]mail.MessageSummary, error) { return nil, nil }
func (noopReader) Read(context.Context, string) (mail.MessageBody, error) { return mail.MessageBody{}, nil }

func main() {
	builder := toolsy.NewRegistryBuilder()
	tools, err := mail.AsTools(noopSender{}, noopReader{})
	if err != nil {
		panic(err)
	}
	for _, tool := range tools {
		builder.Add(tool)
	}
}
```
