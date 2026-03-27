package toolsy

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// WithErrorFormatter converts terminal execution errors from the wrapped tool/middleware
// execution path into an error chunk for LLM self-correction.
//
// Registry/session pre-tool failures (for example ErrToolNotFound, ErrMaxStepsExceeded,
// shutdown, validator rejection) happen before middleware execution and remain hard errors.
func WithErrorFormatter() Middleware {
	return func(next Tool) Tool {
		return &errorFormatterTool{
			toolBase: toolBase{next: next},
		}
	}
}

type errorFormatterTool struct {
	toolBase
}

func (t *errorFormatterTool) Execute(
	ctx context.Context,
	run RunContext,
	input ToolInput,
	yield func(Chunk) error,
) error {
	err := t.next.Execute(ctx, run, input, yield)
	if err == nil {
		return nil
	}
	if shouldBypassErrorFormatting(err) {
		return err
	}

	chunk := newErrorChunk(formatExecutionError(err))
	if chunkErr := validateChunk(chunk); chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(chunk); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}

func shouldBypassErrorFormatting(err error) bool {
	return errors.Is(err, ErrSuspend) ||
		errors.Is(err, ErrStreamAborted) ||
		errors.Is(err, context.Canceled)
}

func formatExecutionError(err error) string {
	var clientErr *ClientError
	if errors.As(err, &clientErr) {
		reason := strings.TrimSpace(clientErr.Reason)
		if reason == "" {
			reason = "tool input is invalid"
		}
		if clientErr.Retryable {
			return "Error executing tool: " + reason + ". Hint: This issue may be transient, retry the same call."
		}
		return "Error executing tool: " + reason + ". Hint: Fix the tool arguments and try again."
	}

	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return "Error executing tool: execution timed out. Hint: Narrow the query or retry later."
	}

	var systemErr *SystemError
	if errors.As(err, &systemErr) {
		return "Error executing tool: internal system error. Hint: Retry later or use a narrower query."
	}

	reason := sanitizeErrorReason(err.Error())
	if reason == "" {
		reason = "request failed"
	}
	return "Error executing tool: " + reason + ". Hint: Retry later or refine the query."
}

func newErrorChunk(message string) Chunk {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "Error executing tool."
	}
	msg = strings.ToValidUTF8(msg, "\uFFFD")
	return Chunk{
		Event:    EventResult,
		Data:     []byte(msg),
		MimeType: MimeTypeText,
		IsError:  true,
	}
}

func sanitizeErrorReason(reason string) string {
	if reason == "" {
		return ""
	}
	reason = strings.ToValidUTF8(reason, "\uFFFD")
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	if idx := strings.IndexAny(reason, "\r\n"); idx >= 0 {
		reason = strings.TrimSpace(reason[:idx])
	}
	if reason == "" {
		return ""
	}

	const maxRunes = 240
	runeCount := utf8.RuneCountInString(reason)
	if runeCount <= maxRunes {
		return reason
	}

	out := make([]rune, 0, maxRunes)
	for _, r := range reason {
		if len(out) >= maxRunes {
			break
		}
		out = append(out, r)
	}
	return string(out)
}
