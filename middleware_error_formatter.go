package toolsy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/toolsy/textprocessor"
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
	prepared, chunkErr := prepareChunk(chunk)
	if chunkErr != nil {
		return chunkErr
	}
	if yieldErr := yield(prepared); yieldErr != nil {
		return wrapYieldError(yieldErr)
	}
	return nil
}

func shouldBypassErrorFormatting(err error) bool {
	return IsControlError(err) ||
		errors.Is(err, ErrStreamAborted) ||
		isContextInterrupt(err)
}

func unwrapInterruptErr(err error) error {
	if te, ok := AsToolError(err); ok && te.Code == CodeInternal && isContextInterrupt(te.Err) {
		return te.Err
	}
	return err
}

func formatExecutionError(err error) string {
	err = unwrapInterruptErr(err)
	if errors.Is(err, context.Canceled) {
		reason := sanitizeErrorReason(err.Error())
		if reason == "" {
			reason = "execution canceled"
		}
		return "Error executing tool: " + reason + ". Hint: Retry later or refine the query."
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return "Error executing tool: execution timed out. Hint: Narrow the query or retry later."
	}

	if textprocessor.IsReadLimitExceeded(err) {
		if n := textprocessor.ReadLimitMaxBytes(err); n > 0 {
			return fmt.Sprintf(
				"Error executing tool: response exceeds %d byte limit. Hint: Narrow the query, reduce payload size, or raise the read budget.",
				n,
			)
		}
		return "Error executing tool: response exceeds byte limit. Hint: Narrow the query, reduce payload size, or raise the read budget."
	}

	if te, ok := AsToolError(err); ok {
		if msg := formatToolErrorMessage(te); msg != "" {
			return msg
		}
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
	if te == nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			te = NewTimeoutErrorFrom(err, true)
		case errors.Is(err, context.Canceled):
			te = NewInternalError(fmt.Errorf("execution canceled: %w", err))
		case isContextInterrupt(err):
			if errors.Is(err, context.DeadlineExceeded) {
				te = NewTimeoutErrorFrom(err, true)
			} else {
				te = NewInternalError(fmt.Errorf("execution canceled: %w", err))
			}
		default:
			te = NewInternalError(errors.New("tool execution failed"))
		}
	}
	llmMessage := errorChunkLLMMessage(te, err)
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
	err = unwrapInterruptErr(err)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewTimeoutErrorFrom(err, true)
	}
	if errors.Is(err, ErrTimeout) {
		return NewTimeoutErrorFrom(ErrTimeout, true)
	}
	if isContextInterrupt(err) {
		return nil
	}
	if te := mapReadLimitForWire(err); te != nil {
		return te
	}
	if te, ok := AsToolError(err); ok {
		return toolErrorFromExistingToolError(te, err)
	}
	return NewInternalError(err)
}

func mapReadLimitForWire(err error) *ToolError {
	if !textprocessor.IsReadLimitExceeded(err) {
		return nil
	}
	if mapped := MapSandboxReadLimitError(err); mapped != nil {
		te, _ := AsToolError(mapped)
		return te
	}
	te, _ := AsToolError(MapReadLimitError(err, 0))
	return te
}

func readLimitToolError(err error) *ToolError {
	return mapReadLimitForWire(err)
}

func errorChunkLLMMessage(te *ToolError, err error) string {
	if te != nil && te.Code == CodeInternal && isContextInterrupt(te.Err) {
		return formatExecutionError(unwrapInterruptErr(te))
	}
	if msg := formatToolErrorMessage(te); msg != "" {
		return msg
	}
	return formatExecutionError(err)
}

func toolErrorFromExistingToolError(te *ToolError, err error) *ToolError {
	err = unwrapInterruptErr(err)
	if isContextInterrupt(err) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrTimeout) {
			return NewTimeoutErrorFrom(err, true)
		}
		return nil
	}
	if mapped := mapReadLimitForWire(err); mapped != nil {
		return mapped
	}
	return te
}

func errorChunkSummaryText(c Chunk, execErr error) string {
	return ErrorChunkSummaryText(c, execErr)
}

// ErrorChunkSummaryText returns a short human-readable summary of an error result chunk.
// Error chunks are normalized first so callers (logging, otel) see the same text as delivered wire.
func ErrorChunkSummaryText(c Chunk, execErr error) string {
	if c.IsError {
		c = normalizeErrorChunk(c)
	}
	if c.MimeType == MimeTypeToolErrorJSON {
		if text := toolErrorWireSummaryText(c.Data); text != "" {
			return text
		}
	}
	// Defensive: non-error text chunks or direct Execute without registry normalization.
	if c.MimeType == MimeTypeText && utf8.Valid(c.Data) {
		return string(c.Data)
	}
	if execErr != nil {
		return formatExecutionError(execErr)
	}
	return ""
}

func toolErrorWireSummaryText(data []byte) string {
	var wire toolErrorWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return ""
	}
	if strings.Contains(wire.Reason, "malformed error chunk") {
		return "Error executing tool: " + sanitizeErrorReason(wire.Reason) + ". Hint: Retry later or refine the query."
	}
	if wire.Message != "" {
		return wire.Message
	}
	te, err := unmarshalToolErrorWire(data)
	if err != nil {
		return ""
	}
	if te.Code == CodeInternal && strings.Contains(te.Reason, "malformed error chunk") {
		return sanitizeErrorReason(te.Reason)
	}
	if msg := errorChunkLLMMessage(te, te.Err); msg != "" {
		return msg
	}
	return te.Reason
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
