package history

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMessage struct {
	Role   string
	Kind   string
	IDs    []string
	Tokens int
	Text   string
}

type testCounter struct{}

func (testCounter) Count(_ context.Context, history []testMessage) (int, error) {
	total := 0
	for _, msg := range history {
		total += msg.Tokens
	}
	return total, nil
}

type errCounter struct{}

func (errCounter) Count(_ context.Context, _ []testMessage) (int, error) {
	return 0, errors.New("count failed")
}

type testSummarizer struct {
	fn func(context.Context, []testMessage) ([]testMessage, error)
}

func (s testSummarizer) Summarize(ctx context.Context, history []testMessage) ([]testMessage, error) {
	if s.fn == nil {
		return nil, errors.New("summarizer fn is nil")
	}
	return s.fn(ctx, history)
}

type testInspector struct{}

func (testInspector) IsSystem(msg testMessage) bool     { return msg.Role == "system" }
func (testInspector) IsToolCall(msg testMessage) bool   { return msg.Kind == "tool_call" }
func (testInspector) IsToolResult(msg testMessage) bool { return msg.Kind == "tool_result" }
func (testInspector) GetToolCallIDs(msg testMessage) []string {
	return msg.IDs
}

func msg(role, kind, text string, tokens int, ids ...string) testMessage {
	return testMessage{
		Role:   role,
		Kind:   kind,
		IDs:    ids,
		Tokens: tokens,
		Text:   text,
	}
}

func TestApplySemanticTruncation_NoOpUnderLimit(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 3),
	}

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			t.Fatal("summarizer must not be called")
			return nil, nil
		}},
		testInspector{},
	)
	require.NoError(t, err)
	assert.Equal(t, history, out)
	assert.False(t, report.Applied)
	assert.Equal(t, 5, report.TokensBefore)
	assert.Equal(t, 5, report.TokensAfter)
}

func TestApplySemanticTruncation_SummarizesOldMessages(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
		msg("user", "regular", "u2", 3),
		msg("assistant", "regular", "a2", 3),
	}

	var summarizedInput []testMessage
	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, in []testMessage) ([]testMessage, error) {
			summarizedInput = append([]testMessage(nil), in...)
			return []testMessage{
				msg("assistant", "summary", "summary", 2),
			}, nil
		}},
		testInspector{},
		WithMinRecentMessages[testMessage](2),
	)
	require.NoError(t, err)

	require.Len(t, summarizedInput, 2)
	assert.Equal(t, []string{"u1", "a1"}, []string{summarizedInput[0].Text, summarizedInput[1].Text})

	assert.Equal(t, []string{"sys", "summary", "u2", "a2"}, []string{
		out[0].Text, out[1].Text, out[2].Text, out[3].Text,
	})
	assert.True(t, report.Applied)
	assert.True(t, report.SummarizationApplied)
	assert.False(t, report.MechanicalTruncationUsed)
	assert.False(t, report.FallbackUsed)
	assert.False(t, report.SummarizerFailed)
	assert.Equal(t, 18, report.TokensBefore)
	assert.Equal(t, 10, report.TokensAfter)
	assert.Equal(t, 1, report.MessagesCompressedCount)
}

func TestApplySemanticTruncation_FallbackOnSummarizerError(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
		msg("user", "regular", "u2", 3),
	}

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return nil, errors.New("llm unavailable")
		}},
		testInspector{},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"sys", "a1", "u2"}, []string{out[0].Text, out[1].Text, out[2].Text})
	assert.True(t, report.Applied)
	assert.True(t, report.FallbackUsed)
	assert.True(t, report.SummarizerFailed)
	assert.True(t, report.MechanicalTruncationUsed)
	assert.False(t, report.SummarizationApplied)
	assert.Equal(t, 10, report.TokensAfter)
}

func TestApplySemanticTruncation_FallbackOnInvalidSummary(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
		msg("user", "regular", "u2", 3),
	}

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return []testMessage{
				msg("assistant", "tool_call", "bad", 1, "x"),
			}, nil
		}},
		testInspector{},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"sys", "a1", "u2"}, []string{out[0].Text, out[1].Text, out[2].Text})
	assert.True(t, report.FallbackUsed)
	assert.True(t, report.SummarizerFailed)
	assert.True(t, report.MechanicalTruncationUsed)
	assert.False(t, report.SummarizationApplied)
}

func TestApplySemanticTruncation_DoubleOverflowUsesMechanicalAfterSummary(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
		msg("user", "regular", "u2", 3),
		msg("assistant", "regular", "a2", 3),
	}

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			// Still too large; post-summary mechanical pass should kick in.
			return []testMessage{
				msg("assistant", "summary", "huge-summary", 8),
			}, nil
		}},
		testInspector{},
		WithMinRecentMessages[testMessage](2),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"sys", "u2", "a2"}, []string{out[0].Text, out[1].Text, out[2].Text})
	assert.True(t, report.Applied)
	assert.True(t, report.MechanicalTruncationUsed)
	assert.False(t, report.FallbackUsed)
	assert.False(t, report.SummarizerFailed)
	assert.False(t, report.SummarizationApplied)
	assert.Equal(t, 8, report.TokensAfter)
}

func TestApplySemanticTruncation_DoesNotSplitToolCallAndResult(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "q", 3),
		msg("assistant", "tool_call", "call", 4, "c1"),
		msg("tool", "tool_result", "result", 4, "c1"),
		msg("assistant", "regular", "final", 3),
	}

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		8,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return nil, errors.New("force fallback")
		}},
		testInspector{},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"sys", "final"}, []string{out[0].Text, out[1].Text})
	assert.True(t, report.FallbackUsed)
	assert.True(t, report.SummarizerFailed)
}

func TestApplySemanticTruncation_ReturnsErrorOnInvalidConfig(t *testing.T) {
	history := []testMessage{msg("system", "regular", "sys", 2)}
	summarizer := testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
		return []testMessage{msg("assistant", "summary", "s", 1)}, nil
	}}

	_, _, err := ApplySemanticTruncation(context.Background(), history, 0, testCounter{}, summarizer, testInspector{})
	require.ErrorContains(t, err, "maxTokens")

	_, _, err = ApplySemanticTruncation(context.Background(), history, 10, nil, summarizer, testInspector{})
	require.ErrorContains(t, err, "token counter is nil")

	_, _, err = ApplySemanticTruncation(context.Background(), history, 10, testCounter{}, nil, testInspector{})
	require.ErrorContains(t, err, "context summarizer is nil")

	_, _, err = ApplySemanticTruncation(context.Background(), history, 10, testCounter{}, summarizer, nil)
	require.ErrorContains(t, err, "message inspector is nil")
}

func TestApplySemanticTruncation_ReturnsErrorWhenProtectedPrefixTooLarge(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "s1", 6),
		msg("system", "regular", "s2", 6),
		msg("user", "regular", "u1", 2),
	}

	_, _, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return []testMessage{msg("assistant", "summary", "s", 1)}, nil
		}},
		testInspector{},
	)
	require.ErrorContains(t, err, "protected system prefix exceeds maxTokens")
}

func TestApplySemanticTruncation_ReturnsErrorWhenTokenCountFails(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
	}

	_, _, err := ApplySemanticTruncation(
		context.Background(),
		history,
		5,
		errCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return []testMessage{msg("assistant", "summary", "s", 1)}, nil
		}},
		testInspector{},
	)
	require.ErrorContains(t, err, "count tokens before truncation")
}

func TestApplySemanticTruncation_ChangedOutputGetsNewBackingArray(t *testing.T) {
	history := []testMessage{
		msg("system", "regular", "sys", 2),
		msg("user", "regular", "u1", 5),
		msg("assistant", "regular", "a1", 5),
		msg("user", "regular", "u2", 3),
	}
	original := append([]testMessage(nil), history...)

	out, report, err := ApplySemanticTruncation(
		context.Background(),
		history,
		10,
		testCounter{},
		testSummarizer{fn: func(_ context.Context, _ []testMessage) ([]testMessage, error) {
			return []testMessage{msg("assistant", "summary", "sum", 2)}, nil
		}},
		testInspector{},
	)
	require.NoError(t, err)
	require.True(t, report.Applied)
	require.NotEmpty(t, out)
	require.NotEmpty(t, history)
	assert.NotSame(t, &history[0], &out[0], "changed output must use a new backing array")
	assert.Equal(t, original, history, "input history must stay unchanged")
}
