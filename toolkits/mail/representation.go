package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/skosovsky/toolsy"
)

// BodyRepresentation describes decoded message text, never a MIME envelope.
type BodyRepresentation string

const (
	BodyPlainText BodyRepresentation = "text/plain"
	BodyHTML      BodyRepresentation = "text/html"
	// BodyMarkdown is an output representation; it is not accepted on MessageBody.
	BodyMarkdown BodyRepresentation = "text/markdown"
)

// ErrBodyRepresentation identifies unsupported input representations.
var ErrBodyRepresentation = errors.New("unsupported mail body representation")

// ErrBodyEncoding identifies invalid UTF-8 in decoded provider body text.
var ErrBodyEncoding = errors.New("mail body is not valid UTF-8")

// BodyRepresentationError retains the unsupported host declaration.
type BodyRepresentationError struct{ Representation BodyRepresentation }

func (e *BodyRepresentationError) Error() string {
	return fmt.Sprintf("%v: %q", ErrBodyRepresentation, e.Representation)
}
func (e *BodyRepresentationError) Unwrap() error { return ErrBodyRepresentation }

func normalizeBody(
	ctx context.Context,
	body string,
	representation BodyRepresentation,
) (string, BodyRepresentation, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	if !utf8.ValidString(body) {
		return "", "", bodyFailure("mail_body_encoding", ErrBodyEncoding)
	}
	switch representation {
	case "", BodyPlainText:
		return body, BodyPlainText, nil
	case BodyHTML:
		return convertHTMLBody(ctx, body, func(s string) (string, error) { return htmltomarkdown.ConvertString(s) })
	case BodyMarkdown:
		return "", "", bodyFailure("mail_body_representation", &BodyRepresentationError{Representation: representation})
	default:
		return "", "", bodyFailure("mail_body_representation", &BodyRepresentationError{Representation: representation})
	}
}

// The converter is synchronous; cancellation is observed around its call.
// Passing the converter explicitly keeps failure testing independent of global hooks.
func convertHTMLBody(
	ctx context.Context,
	body string,
	convert func(string) (string, error),
) (string, BodyRepresentation, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	md, err := convert(body)
	if interrupt := ctx.Err(); interrupt != nil {
		return "", "", interrupt
	}
	if err != nil {
		return "", "", bodyFailure("mail_body_conversion", err)
	}
	if !utf8.ValidString(md) {
		return "", "", bodyFailure("mail_body_encoding", ErrBodyEncoding)
	}
	return strings.TrimSpace(md), BodyMarkdown, nil
}

func bodyFailure(kind string, cause error) error {
	return toolsy.NewInternalError(&toolsy.ResultContractError{Kind: kind, Cause: cause})
}
