package agents

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

// Outcome identifies what is known about remote business completion.
type Outcome string

const (
	// PhaseCreate marks an action whose acceptance may be unknown.
	PhaseCreate = "create"
	// PhaseObserve marks read failure after a known task reference.
	PhaseObserve              = "observe"
	OutcomeFailed     Outcome = "failed"
	OutcomeCancelled  Outcome = "cancelled"
	OutcomeIncomplete Outcome = "incomplete"
	OutcomeMalformed  Outcome = "malformed"
	OutcomeUnknown    Outcome = "unknown"
)

// RemoteOutcomeError preserves a task reference and partial observation without
// declaring success or granting permission to repeat task creation.
type RemoteOutcomeError struct {
	// Phase is create or observe; create errors may have no recoverable task ID.
	Phase   string
	Outcome Outcome
	TaskID  string
	Step    *Step
	Cause   error
}

func (e *RemoteOutcomeError) Error() string {
	// Task IDs, remote output and transport text are untrusted; inspect fields explicitly.
	return fmt.Sprintf("agents: remote outcome %s", e.Outcome)
}

func (e *RemoteOutcomeError) Unwrap() error { return e.Cause }

// AcceptedTaskReference acknowledges creation only. The host owns persistence,
// status retrieval and continuation; Accepted never means business completion.
type AcceptedTaskReference struct {
	TaskID   string `json:"task_id"`
	Accepted bool   `json:"accepted"`
}

func validateStep(step Step) error {
	if step.TaskID == "" || step.StepID == "" {
		return &RemoteOutcomeError{
			Phase:   PhaseObserve,
			Outcome: OutcomeMalformed,
			TaskID:  step.TaskID,
			Step:    &step,
			Cause:   nil,
		}
	}
	switch step.Status {
	case "created", "running":
		if !step.IsLast {
			return nil
		}
	case "completed":
		return nil
	case "failed", "cancelled":
		if step.IsLast {
			return nil
		}
	}
	return &RemoteOutcomeError{
		Phase:   PhaseObserve,
		Outcome: OutcomeMalformed,
		TaskID:  step.TaskID,
		Step:    &step,
		Cause:   nil,
	}
}

func interruptionOutcome(remote *RemoteOutcomeError) error {
	if errors.Is(remote.Cause, context.DeadlineExceeded) || errors.Is(remote.Cause, toolsy.ErrTimeout) {
		return toolsy.NewTimeoutErrorFrom(remote, false)
	}
	return remote
}
