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
	env *RunEnv,
	input ToolInput,
	yield func(Chunk) error,
) error {
	err := t.next.Execute(ctx, env, input, yield)
	if err == nil {
		return nil
	}
	if shouldBypassErrorFormatting(err) {
		return err
	}

	chunk := NewErrorChunkFromErr(err)
	if chunkErr := validateChunk(chunk); chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(chunk); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}

func shouldBypassErrorFormatting(err error) bool {
	return IsControlError(err) ||
		errors.Is(err, ErrStreamAborted) ||
		errors.Is(err, context.Canceled)
}

func formatExecutionError(err error) string {
	if te, ok := AsToolError(err); ok {
		if msg := formatToolErrorMessage(te); msg != "" {
			return msg
		}
	}

	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return "Error executing tool: execution timed out. Hint: Narrow the query or retry later."
	}

	reason := sanitizeErrorReason(err.Error())
	if reason == "" {
		reason = "request failed"
	}
	return "Error executing tool: " + reason + ". Hint: Retry later or refine the query."
}

func formatToolErrorMessage(te *ToolError) string {
	reason := strings.TrimSpace(te.SafeMessage)
	if reason == "" {
		reason = strings.TrimSpace(te.Reason)
	}
	if ClientCorrectable(te.Code) {
		if reason == "" {
			reason = "tool input is invalid"
		}
		if te.Retryable {
			return "Error executing tool: " + reason + ". Hint: This issue may be transient, retry the same call."
		}
		return "Error executing tool: " + reason + ". Hint: Fix the tool arguments and try again."
	}
	if orchestratorSystemCode(te.Code) {
		return "Error executing tool: internal system error. Hint: Retry later or use a narrower query."
	}
	if te.Code == CodeDependencyMissing {
		return "Error executing tool: required dependency is missing. Hint: Check agent configuration and session wiring."
	}
	if te.Code == CodeToolsContractMissing {
		return "Error executing tool: required tools are not registered. Hint: Register missing tools or adjust the contract."
	}
	if te.Code == CodeBudgetExceeded {
		return "Error executing tool: " + sanitizeErrorReason(te.Reason) +
			". Hint: Narrow the query or reduce tool usage."
	}
	if reason == "" {
		return ""
	}
	return "Error executing tool: " + sanitizeErrorReason(reason) + ". Hint: Retry later or refine the query."
}

// NewErrorChunkFromErr builds a structured error result chunk with [MimeTypeToolErrorJSON].
func NewErrorChunkFromErr(err error) Chunk {
	te := toolErrorFromExecutionErr(err)
	llmMessage := formatToolErrorMessage(te)
	if llmMessage == "" {
		llmMessage = formatExecutionError(err)
	}
	data, marshalErr := marshalToolErrorWire(te, llmMessage)
	if marshalErr != nil {
		te = NewInternalError(marshalErr)
		llmMessage = formatExecutionError(marshalErr)
		data, _ = marshalToolErrorWire(te, llmMessage)
	}
	return Chunk{
		Event:    EventResult,
		Data:     data,
		MimeType: MimeTypeToolErrorJSON,
		IsError:  true,
	}
}

func toolErrorFromExecutionErr(err error) *ToolError {
	if err == nil {
		return NewInternalError(errors.New("tool execution failed"))
	}
	if te, ok := AsToolError(err); ok {
		return te
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return NewTimeoutError(true)
	}
	return NewInternalError(err)
}

func errorChunkSummaryText(c Chunk, execErr error) string {
	if c.MimeType == MimeTypeToolErrorJSON {
		if te, err := unmarshalToolErrorWire(c.Data); err == nil {
			if msg := formatToolErrorMessage(te); msg != "" {
				return msg
			}
			return te.Reason
		}
	}
	if c.MimeType == MimeTypeText && utf8.Valid(c.Data) {
		return string(c.Data)
	}
	if execErr != nil {
		return formatExecutionError(execErr)
	}
	return ""
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
