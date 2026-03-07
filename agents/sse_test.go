package agents

import (
	"strings"
	"testing"
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
	lastID, done, yieldedAny, err := parseSSESteps(r, yield)
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
	lastID, done, _, err := parseSSESteps(r, yield)
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
	lastID, _, _, err := parseSSESteps(r, yield)
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
