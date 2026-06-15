package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/httptool"
)

func TestParseSSESteps_MultiLineData(t *testing.T) {
	// One Step JSON split across multiple data: lines must be concatenated with newline and parsed as one event.
	// Split at a boundary where newline is valid JSON whitespace (between tokens), not inside a string.
	raw := "id: ev1\n" +
		"data: {\"step_id\":\"s1\",\n" +
		"data: \"task_id\":\"t1\",\"name\":\"step_one\",\"status\":\"completed\",\"is_last\":true}\n\n"
	r := strings.NewReader(raw)
	var steps []Step
	yield := func(s Step, _ error) bool {
		steps = append(steps, s)
		return true
	}
	lastID, done, yieldedAny, err := parseSSESteps(context.Background(), r, httptool.DefaultMaxSSEStreamBytes, yield)
	if err != nil {
		t.Fatalf("parseSSESteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].StepID != "s1" || steps[0].TaskID != "t1" || steps[0].Name != "step_one" {
		t.Errorf("step: got step_id=%q task_id=%q name=%q", steps[0].StepID, steps[0].TaskID, steps[0].Name)
	}
	if !steps[0].IsLast {
		t.Error("expected is_last true")
	}
	if lastID != "ev1" {
		t.Errorf("lastID = %q, want ev1", lastID)
	}
	if !done {
		t.Error("expected done true")
	}
	if !yieldedAny {
		t.Error("expected yieldedAny true")
	}
}

func TestParseSSESteps_LastEventID(t *testing.T) {
	// Multiple events: lastID must be the id of the last processed event.
	raw := "id: first\n" +
		"data: {\"step_id\":\"a\",\"task_id\":\"t\",\"name\":\"n\",\"status\":\"running\",\"is_last\":false}\n\n" +
		"id: second\n" +
		"data: {\"step_id\":\"b\",\"task_id\":\"t\",\"name\":\"n\",\"status\":\"completed\",\"is_last\":true}\n\n"
	r := strings.NewReader(raw)
	var steps []Step
	yield := func(s Step, _ error) bool {
		steps = append(steps, s)
		return true
	}
	lastID, done, _, err := parseSSESteps(context.Background(), r, httptool.DefaultMaxSSEStreamBytes, yield)
	if err != nil {
		t.Fatalf("parseSSESteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if lastID != "second" {
		t.Errorf("lastID = %q, want second", lastID)
	}
	if !done {
		t.Error("expected done true")
	}
}

func TestParseSSESteps_EventTypeIgnored(t *testing.T) {
	// event: line is optional and should not break parsing; id and data are used.
	raw := "event: step\n" +
		"id: ev1\n" +
		"data: {\"step_id\":\"s1\",\"task_id\":\"t1\",\"name\":\"n\",\"status\":\"completed\",\"is_last\":true}\n\n"
	r := strings.NewReader(raw)
	var steps []Step
	yield := func(s Step, _ error) bool {
		steps = append(steps, s)
		return true
	}
	lastID, _, _, err := parseSSESteps(context.Background(), r, httptool.DefaultMaxSSEStreamBytes, yield)
	if err != nil {
		t.Fatalf("parseSSESteps: %v", err)
	}
	if len(steps) != 1 || steps[0].StepID != "s1" {
		t.Errorf("expected one step step_id=s1, got %d steps", len(steps))
	}
	if lastID != "ev1" {
		t.Errorf("lastID = %q, want ev1", lastID)
	}
}

func TestParseSSESteps_StreamExceedsMaxBytes(t *testing.T) {
	t.Parallel()
	// Valid JSON large enough to exceed the stream byte cap while reading.
	payload := strings.Repeat("x", 3000)
	raw := "id: ev1\n" +
		"data: {\"step_id\":\"s1\",\"task_id\":\"t1\",\"name\":\"" + payload + "\",\"status\":\"running\",\"is_last\":false}\n\n"
	const streamCap = 2048
	limited := httptool.LimitStreamReaderWithContext(context.Background(), strings.NewReader(raw), streamCap)
	_, _, _, err := parseSSESteps(context.Background(), limited, streamCap, func(Step, error) bool { return true })
	require.Error(t, err)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "2048")
}

func TestParseSSESteps_CancelOverStreamLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	payload := strings.Repeat("x", 3000)
	raw := "id: ev1\n" +
		"data: {\"step_id\":\"s1\",\"task_id\":\"t1\",\"name\":\"" + payload + "\",\"status\":\"running\",\"is_last\":false}\n\n"
	const streamCap = 2048
	limited := httptool.LimitStreamReaderWithContext(ctx, strings.NewReader(raw), streamCap)
	cancel()
	_, _, _, err := parseSSESteps(ctx, limited, streamCap, func(Step, error) bool { return true })
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseSSESteps_InterruptInChainOverReadLimit(t *testing.T) {
	t.Parallel()
	composite := fmt.Errorf(
		"agents: stream: %w",
		errors.Join(context.Canceled, textprocessor.ErrReadLimitExceeded),
	)
	raw := "id: ev1\n" +
		"data: {\"step_id\":\"s1\",\"task_id\":\"t1\",\"name\":\"x\",\"status\":\"running\",\"is_last\":true}\n\n"
	stream := io.MultiReader(strings.NewReader(raw), &instantErrReader{err: composite})
	_, _, _, err := parseSSESteps(context.Background(), stream, 1<<20, func(Step, error) bool { return true })
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

type instantErrReader struct {
	err error
}

func (r *instantErrReader) Read([]byte) (int, error) {
	return 0, r.err
}
