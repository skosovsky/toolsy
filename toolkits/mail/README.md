# Toolsy: Mail Toolkit (mail)

**Description:** Lets the agent send email and read/search inbox via MailSender and MailReader interfaces. Implementations can use SMTP, IMAP, SendGrid, Resend, Gmail API, etc.

## Installation

```bash
go get github.com/skosovsky/toolsy/toolkits/mail
```

**Dependencies:** `github.com/skosovsky/toolsy`, `github.com/JohannesKaufmann/html-to-markdown/v2` (for HTML body normalization).

## Available tools

| Tool                | Description                 | Input                                                       |
| ------------------- | --------------------------- | ----------------------------------------------------------- |
| `mail_send`         | Send an email               | `{"to": ["string"], "subject": "string", "body": "string"}` |
| `mail_search_inbox` | Search inbox by query       | `{"query": "string", "limit": int}`                         |
| `mail_read_message` | Read a single message by ID | `{"message_id": "string"}`                                  |

Tools are generated only when the corresponding interface is provided: nil sender skips mail_send; nil reader skips both search and read. At least one must be non-nil. Body in `mail_read_message` is converted to Markdown only when it looks like HTML (tag-like patterns); plain text with `<` (e.g. "x < 5") is left as-is. HTML conversion checks cancellation before and after conversion; the converter itself is synchronous. `message_id` is trimmed; whitespace-only is rejected with `CodeValidationFailed` ([`ToolError`](../../errors.go)).

## Configuration & Security

> **Warning:** Credentials and network access are the responsibility of your MailSender/MailReader implementations. Use read-only or nil sender in production if the agent must not send mail.

- **Nil-safe:** Pass `nil` for sender to get only search/read tools; pass `nil` for reader to get only send. At least one must be non-nil.
- **WithReadOnly(true):** Disables mail_send even when sender is non-nil.
- **Limits:** Finite defaults: body 256 KiB, aggregate source strings 1 MiB, one message/item 256 KiB, search count 100, final encoded JSON 1 MiB. Host options are `WithMaxBodyBytes`, `WithMaxSourceBytes`, `WithMaxItemBytes`, `WithMaxSearchResults`, `WithMaxWireBytes`. Zero selects the default; negative is a construction error. No option disables limits.
- **Send:** Oversize body is a validation error before `Send`. Accepted recipients, subject and body reach the provider unchanged. The fixed success response is checked before dispatch. Tool flags describe risk; the host must bind approval to prepared execution through its operation profile. No toolkit normalization changes approved action arguments.
- **Read/search:** Source/count/item bounds check actual provider results before formatting; final bounds check JSON after escaping. Oversize results fail explicitly; there is no silent truncation. Search `limit=0` defaults to min(10, host count cap); negative or above cap is validation error. A provider returning more than requested is rejected.
- **Continuation:** The injected reader exposes no range/cursor contract, so this toolkit does not invent continuation tokens. Narrow the query or request a smaller message through your backend. Source checks happen after provider allocation; host readers must enforce transport/allocation limits and cancellation themselves. HTML conversion has bounded input, but no hard CPU deadline or intermediate-allocation guarantee. Email/HTML content is untrusted data.
- **Query required:** Empty or whitespace-only `query` in mail_search_inbox returns validation `ToolError` (avoids dumping entire inbox). Same for `message_id` in mail_read_message.

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
