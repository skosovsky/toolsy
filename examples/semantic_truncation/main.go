// Package main demonstrates semantic chat-history truncation with BYOT contracts.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/skosovsky/toolsy/history"
)

const demoMaxTokens = 23

type MyMessage struct {
	Role    string
	Kind    string
	CallIDs []string
	Content string
}

type myCounter struct{}

func (myCounter) Count(_ context.Context, msgs []MyMessage) (int, error) {
	total := 0
	for _, m := range msgs {
		total += len(strings.Fields(m.Content))
	}
	return total, nil
}

type mySummarizer struct{}

func (mySummarizer) Summarize(_ context.Context, msgs []MyMessage) ([]MyMessage, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	return []MyMessage{
		{
			Role:    "assistant",
			Kind:    "summary",
			Content: "Summary: weather requested.",
		},
	}, nil
}

type myInspector struct{}

func (myInspector) IsSystem(m MyMessage) bool     { return m.Role == "system" }
func (myInspector) IsToolCall(m MyMessage) bool   { return m.Kind == "tool_call" }
func (myInspector) IsToolResult(m MyMessage) bool { return m.Kind == "tool_result" }
func (myInspector) GetToolCallIDs(m MyMessage) []string {
	return m.CallIDs
}

func main() {
	msgs := []MyMessage{
		{Role: "system", Kind: "regular", Content: "You are a helpful assistant."},
		{Role: "user", Kind: "regular", Content: "Find weather in Moscow for tomorrow morning."},
		{Role: "assistant", Kind: "tool_call", CallIDs: []string{"call-1"}, Content: "Calling weather API"},
		{Role: "tool", Kind: "tool_result", CallIDs: []string{"call-1"}, Content: "Weather API returned 18C"},
		{Role: "assistant", Kind: "regular", Content: "It will be around 18C with light wind."},
	}

	out, report, err := history.ApplySemanticTruncation(
		context.Background(),
		msgs,
		demoMaxTokens,
		myCounter{},
		mySummarizer{},
		myInspector{},
		history.WithMinRecentMessages[MyMessage](2),
	)
	if err != nil {
		log.Fatalf("semantic truncation: %v", err)
	}

	_, _ = fmt.Fprintf(
		os.Stdout,
		"applied=%v fallback=%v before=%d after=%d\n",
		report.Applied,
		report.FallbackUsed,
		report.TokensBefore,
		report.TokensAfter,
	)
	for i, m := range out {
		_, _ = fmt.Fprintf(os.Stdout, "%d. %s/%s: %s\n", i+1, m.Role, m.Kind, m.Content)
	}
}
