package history

import (
	"context"
	"errors"
	"testing"
)

// BenchmarkSemanticTruncationLongHistory measures a 4,001-message history with
// paired tool calls/results. The host counter visits every candidate message.
func BenchmarkSemanticTruncationLongHistory(b *testing.B) {
	history := []testMessage{msg("system", "regular", "instructions", 2)}
	for range 2000 {
		history = append(
			history,
			msg("assistant", "tool_call", "call", 4, "call"),
			msg("tool", "tool_result", "result", 4, "call"),
		)
	}
	for _, fail := range []bool{false, true} {
		name := "summary"
		if fail {
			name = "fallback"
		}
		b.Run(name, func(b *testing.B) {
			counter := &benchmarkHistoryCounter{}
			summarizer := testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
				if fail {
					return nil, errors.New("summary unavailable")
				}
				return []testMessage{msg("assistant", "summary", "summary", 2)}, nil
			}}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _, err := ApplySemanticTruncation(
					context.Background(),
					history,
					100,
					counter,
					summarizer,
					testInspector{},
					WithMinRecentMessages[testMessage](2),
				)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(counter.visited)/float64(b.N), "messages-counted/op")
			b.ReportMetric(float64(counter.calls)/float64(b.N), "count-calls/op")
		})
	}
}

type benchmarkHistoryCounter struct{ visited, calls int }

func (c *benchmarkHistoryCounter) Count(_ context.Context, history []testMessage) (int, error) {
	c.calls++
	c.visited += len(history)
	return (testCounter{}).Count(context.Background(), history)
}
