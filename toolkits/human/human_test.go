package human

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func assertErrorContainsToolkitHuman(t *testing.T, err error) {
	t.Helper()
	for e := err; e != nil; e = errors.Unwrap(e) {
		if strings.Contains(e.Error(), "toolkit/human:") {
			return
		}
	}
	require.Fail(t, "expected toolkit/human in error chain", "got: %v", err)
}

// mockHandler records last arguments and returns configured values.
type mockHandler struct {
	approved     bool
	answer       string
	err          error
	lastAction   string
	lastReason   string
	lastQuestion string
}

func (m *mockHandler) ApproveAction(ctx context.Context, action, reason string) (bool, error) {
	_ = ctx
	m.lastAction = action
	m.lastReason = reason
	if m.err != nil {
		return false, m.err
	}
	return m.approved, nil
}

func (m *mockHandler) ProvideClarification(ctx context.Context, question string) (string, error) {
	_ = ctx
	m.lastQuestion = question
	if m.err != nil {
		return "", m.err
	}
	return m.answer, nil
}

func TestRequestApproval_Approved(t *testing.T) {
	h := &mockHandler{approved: true}
	tools, err := AsTools(h)
	require.NoError(t, err)
	approvalTool := tools[0]

	var decision string
	require.NoError(
		t,
		approvalTool.Execute(
			context.Background(),
			[]byte(`{"action":"delete","reason":"user asked"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(approvalResult); ok {
						decision = r.Decision
					}
				}
				return nil
			},
		),
	)
	require.Equal(t, "APPROVED", decision)
}

func TestRequestApproval_Rejected(t *testing.T) {
	h := &mockHandler{approved: false}
	tools, err := AsTools(h)
	require.NoError(t, err)
	approvalTool := tools[0]

	var decision string
	require.NoError(
		t,
		approvalTool.Execute(
			context.Background(),
			[]byte(`{"action":"delete","reason":"test"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(approvalResult); ok {
						decision = r.Decision
					}
				}
				return nil
			},
		),
	)
	require.Equal(t, "REJECTED", decision)
}

func TestRequestApproval_ArgsPassedCorrectly(t *testing.T) {
	h := &mockHandler{approved: true}
	tools, err := AsTools(h)
	require.NoError(t, err)
	approvalTool := tools[0]

	require.NoError(
		t,
		approvalTool.Execute(
			context.Background(),
			[]byte(`{"action":"send_email","reason":"user requested"}`),
			func(toolsy.Chunk) error { return nil },
		),
	)
	require.Equal(t, "send_email", h.lastAction)
	require.Equal(t, "user requested", h.lastReason)
}

func TestRequestApproval_HandlerError(t *testing.T) {
	h := &mockHandler{err: errors.New("timeout")}
	tools, err := AsTools(h)
	require.NoError(t, err)
	approvalTool := tools[0]

	err = approvalTool.Execute(
		context.Background(),
		[]byte(`{"action":"x","reason":"y"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	assertErrorContainsToolkitHuman(t, err)
}

func TestAskClarification_ReturnsAnswer(t *testing.T) {
	h := &mockHandler{answer: "Use the blue button"}
	tools, err := AsTools(h)
	require.NoError(t, err)
	clarificationTool := tools[1]

	var answer string
	require.NoError(
		t,
		clarificationTool.Execute(
			context.Background(),
			[]byte(`{"question":"Which button?"}`),
			func(c toolsy.Chunk) error {
				if c.RawData != nil {
					if r, ok := c.RawData.(clarificationResult); ok {
						answer = r.Answer
					}
				}
				return nil
			},
		),
	)
	require.Equal(t, "Use the blue button", answer)
}

func TestAskClarification_ArgsPassedCorrectly(t *testing.T) {
	h := &mockHandler{answer: "ok"}
	tools, err := AsTools(h)
	require.NoError(t, err)
	clarificationTool := tools[1]

	require.NoError(
		t,
		clarificationTool.Execute(
			context.Background(),
			[]byte(`{"question":"What is the deadline?"}`),
			func(toolsy.Chunk) error { return nil },
		),
	)
	require.Equal(t, "What is the deadline?", h.lastQuestion)
}

func TestAskClarification_HandlerError(t *testing.T) {
	h := &mockHandler{err: errors.New("cancelled")}
	tools, err := AsTools(h)
	require.NoError(t, err)
	clarificationTool := tools[1]

	err = clarificationTool.Execute(
		context.Background(),
		[]byte(`{"question":"x"}`),
		func(toolsy.Chunk) error { return nil },
	)
	require.Error(t, err)
	assertErrorContainsToolkitHuman(t, err)
}

func TestAsTools_ToolCount(t *testing.T) {
	tools, err := AsTools(&mockHandler{})
	require.NoError(t, err)
	require.Len(t, tools, 2)
}

func TestAsTools_CustomNames(t *testing.T) {
	tools, err := AsTools(&mockHandler{}, WithApprovalName("approve"), WithClarificationName("clarify"))
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "approve", tools[0].Name())
	require.Equal(t, "clarify", tools[1].Name())
}
