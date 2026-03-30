package history

import (
	"context"
	"fmt"
)

// TokenCounter counts token usage for a history snapshot.
type TokenCounter[T any] interface {
	Count(ctx context.Context, history []T) (int, error)
}

// ContextSummarizer compresses older history into one or more summary messages.
type ContextSummarizer[T any] interface {
	Summarize(ctx context.Context, history []T) ([]T, error)
}

// MessageInspector exposes semantic boundaries of user-defined message type T.
type MessageInspector[T any] interface {
	IsSystem(msg T) bool
	IsToolCall(msg T) bool
	IsToolResult(msg T) bool
	GetToolCallIDs(msg T) []string
}

// SemanticTruncationOption configures semantic truncation behavior.
type SemanticTruncationOption[T any] func(*semanticTruncationConfig[T])

type semanticTruncationConfig[T any] struct {
	minRecentMessages int
}

// WithMinRecentMessages keeps at least n most-recent raw messages untouched during summary selection.
// A later mechanical pass may still trim below this value to satisfy maxTokens.
func WithMinRecentMessages[T any](n int) SemanticTruncationOption[T] {
	return func(c *semanticTruncationConfig[T]) {
		if n >= 0 {
			c.minRecentMessages = n
		}
	}
}

// SemanticTruncationReport contains structured outcome metadata for observability.
type SemanticTruncationReport struct {
	Applied                  bool
	SummarizationApplied     bool
	MechanicalTruncationUsed bool
	FallbackUsed             bool
	SummarizerFailed         bool
	TokensBefore             int
	TokensAfter              int
	MessagesCompressedCount  int
}

// ApplySemanticTruncation compresses old history while preserving recent messages and tool-call boundaries.
//
// The function is pure at slice-structure level:
//   - if no changes are needed, it may return the original history slice;
//   - if changes are applied, it allocates a new backing array for the returned slice.
func ApplySemanticTruncation[T any](
	ctx context.Context,
	history []T,
	maxTokens int,
	counter TokenCounter[T],
	summarizer ContextSummarizer[T],
	inspector MessageInspector[T],
	opts ...SemanticTruncationOption[T],
) ([]T, SemanticTruncationReport, error) {
	var report SemanticTruncationReport

	if maxTokens <= 0 {
		return nil, report, fmt.Errorf("toolsy/history: maxTokens must be > 0")
	}
	if counter == nil {
		return nil, report, fmt.Errorf("toolsy/history: token counter is nil")
	}
	if summarizer == nil {
		return nil, report, fmt.Errorf("toolsy/history: context summarizer is nil")
	}
	if inspector == nil {
		return nil, report, fmt.Errorf("toolsy/history: message inspector is nil")
	}

	cfg := semanticTruncationConfig[T]{minRecentMessages: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.minRecentMessages < 0 {
		cfg.minRecentMessages = 0
	}

	beforeTokens, err := counter.Count(ctx, history)
	if err != nil {
		return nil, report, fmt.Errorf("toolsy/history: count tokens before truncation: %w", err)
	}
	report.TokensBefore = beforeTokens
	report.TokensAfter = beforeTokens

	if beforeTokens <= maxTokens {
		return history, report, nil
	}

	protectedEnd := leadingSystemPrefixLen(history, inspector)
	protectedPrefix := history[:protectedEnd]

	protectedTokens, err := counter.Count(ctx, protectedPrefix)
	if err != nil {
		return nil, report, fmt.Errorf("toolsy/history: count protected prefix tokens: %w", err)
	}
	if protectedTokens > maxTokens {
		return nil, report, fmt.Errorf("toolsy/history: protected system prefix exceeds maxTokens")
	}

	safeBoundaries := computeSafeBoundaries(history, protectedEnd, inspector)
	mechBoundary, mechTokensAfter, err := findMechanicalBoundary(
		ctx,
		history,
		protectedPrefix,
		safeBoundaries,
		maxTokens,
		counter,
	)
	if err != nil {
		return nil, report, err
	}

	summaryBoundary := chooseSummaryBoundary(
		safeBoundaries,
		protectedEnd,
		len(history),
		cfg.minRecentMessages,
	)

	if summaryBoundary > protectedEnd {
		compressPart := cloneSlice(history[protectedEnd:summaryBoundary])
		summary, sumErr := summarizer.Summarize(ctx, compressPart)
		if sumErr == nil && isValidSummary(summary, inspector) {
			suffix := history[summaryBoundary:]
			summaryOut := joinSegments(protectedPrefix, summary, suffix)

			summaryTokens, countErr := counter.Count(ctx, summaryOut)
			if countErr != nil {
				return nil, report, fmt.Errorf("toolsy/history: count summary output tokens: %w", countErr)
			}

			if summaryTokens <= maxTokens {
				report.Applied = !sameSliceData(history, summaryOut)
				report.SummarizationApplied = len(summary) > 0
				report.TokensAfter = summaryTokens
				report.MessagesCompressedCount = compressedCount(len(history), len(summaryOut))
				return summaryOut, report, nil
			}

			// Double-overflow: force a boundary-safe mechanical pass over the summary output.
			postSafe := computeSafeBoundaries(summaryOut, protectedEnd, inspector)
			postBoundary, postTokensAfter, postErr := findMechanicalBoundary(
				ctx,
				summaryOut,
				protectedPrefix,
				postSafe,
				maxTokens,
				counter,
			)
			if postErr != nil {
				return nil, report, postErr
			}

			postSuffix := summaryOut[postBoundary:]
			finalOut := joinSegments(protectedPrefix, nil, postSuffix)

			summaryStart := protectedEnd
			summaryEnd := protectedEnd + len(summary)
			keptSummary := len(summary) > 0 && postBoundary < summaryEnd && len(finalOut) > summaryStart

			report.Applied = !sameSliceData(history, finalOut)
			report.SummarizationApplied = keptSummary
			report.MechanicalTruncationUsed = true
			report.TokensAfter = postTokensAfter
			report.MessagesCompressedCount = compressedCount(len(history), len(finalOut))
			return finalOut, report, nil
		}

		report.SummarizerFailed = true
		report.FallbackUsed = true
	}

	// Fallback: boundary-safe mechanical truncation on original history.
	finalSuffix := history[mechBoundary:]
	out := joinSegments(protectedPrefix, nil, finalSuffix)

	report.Applied = !sameSliceData(history, out)
	report.SummarizationApplied = false
	report.MechanicalTruncationUsed = true
	report.TokensAfter = mechTokensAfter
	report.MessagesCompressedCount = compressedCount(len(history), len(out))
	return out, report, nil
}

func leadingSystemPrefixLen[T any](history []T, inspector MessageInspector[T]) int {
	i := 0
	for i < len(history) && inspector.IsSystem(history[i]) {
		i++
	}
	return i
}

func computeSafeBoundaries[T any](history []T, protectedEnd int, inspector MessageInspector[T]) []int {
	if protectedEnd < 0 {
		protectedEnd = 0
	}
	if protectedEnd > len(history) {
		protectedEnd = len(history)
	}

	idsOpen := make(map[string]int)
	boundaries := make([]int, 0, len(history)-protectedEnd+2)
	boundaries = append(boundaries, protectedEnd)

	for i := protectedEnd; i < len(history); i++ {
		msg := history[i]
		ids := inspector.GetToolCallIDs(msg)
		if inspector.IsToolCall(msg) {
			for _, id := range ids {
				if id == "" {
					continue
				}
				idsOpen[id]++
			}
		}
		if inspector.IsToolResult(msg) {
			for _, id := range ids {
				if id == "" {
					continue
				}
				if n, ok := idsOpen[id]; ok {
					if n <= 1 {
						delete(idsOpen, id)
					} else {
						idsOpen[id] = n - 1
					}
				}
			}
		}
		if len(idsOpen) == 0 {
			boundaries = append(boundaries, i+1)
		}
	}

	if len(boundaries) == 0 || boundaries[len(boundaries)-1] != len(history) {
		boundaries = append(boundaries, len(history))
	}
	return boundaries
}

func findMechanicalBoundary[T any](
	ctx context.Context,
	history []T,
	protectedPrefix []T,
	safeBoundaries []int,
	maxTokens int,
	counter TokenCounter[T],
) (boundary int, tokensAfter int, err error) {
	for _, b := range safeBoundaries {
		suffix := history[b:]
		candidate := joinSegments(protectedPrefix, nil, suffix)
		toks, countErr := counter.Count(ctx, candidate)
		if countErr != nil {
			return 0, 0, fmt.Errorf("toolsy/history: count tokens for mechanical candidate: %w", countErr)
		}
		if toks <= maxTokens {
			return b, toks, nil
		}
	}
	return 0, 0, fmt.Errorf("toolsy/history: no boundary-safe truncation candidate fits maxTokens")
}

func chooseSummaryBoundary(
	safeBoundaries []int,
	protectedEnd, totalLen, minRecent int,
) int {
	if protectedEnd >= totalLen {
		return protectedEnd
	}

	targetMax := totalLen - minRecent
	if targetMax < protectedEnd+1 {
		targetMax = protectedEnd + 1
	}

	best := protectedEnd
	for _, b := range safeBoundaries {
		if b <= protectedEnd {
			continue
		}
		if b <= targetMax && b > best {
			best = b
		}
	}
	if best > protectedEnd {
		return best
	}

	for i := len(safeBoundaries) - 1; i >= 0; i-- {
		b := safeBoundaries[i]
		if b > protectedEnd {
			return b
		}
	}
	return protectedEnd
}

func isValidSummary[T any](summary []T, inspector MessageInspector[T]) bool {
	if len(summary) == 0 {
		return false
	}
	for _, msg := range summary {
		if inspector.IsToolCall(msg) || inspector.IsToolResult(msg) {
			return false
		}
	}
	return true
}

func joinSegments[T any](prefix, middle, suffix []T) []T {
	out := make([]T, 0, len(prefix)+len(middle)+len(suffix))
	out = append(out, prefix...)
	out = append(out, middle...)
	out = append(out, suffix...)
	return out
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func sameSliceData[T any](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
}

func compressedCount(beforeLen, afterLen int) int {
	if beforeLen > afterLen {
		return beforeLen - afterLen
	}
	return 0
}
