// host_dispatch is a provider-neutral host application with a sequential barrier.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/examples/host_dispatch/recipe"
)

const recordTool = "record"

type subject struct{ ID string }
type scope struct{ ID string }
type activity struct{ Key string }
type args struct {
	ID int64 `json:"id"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

//nolint:funlen // Keep the standalone approval, execution and redelivery walkthrough linear.
func run() error {
	ctx := context.Background()
	store := toolsy.NewMemoryOperationStore()
	profile, err := recipe.Profile[subject, scope, activity](
		store,
		toolsy.JSONResultCodec[args, string]{},
		"host",
		time.Now,
		func(s subject, c scope) (string, string, error) { return s.ID, c.ID, nil },
	)
	if err != nil {
		return err
	}
	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[subject, scope, args, args, string]{
		Name:        recordTool,
		Description: "Record an exact identifier",
		Options:     []toolsy.ToolOption{toolsy.WithRequiresConfirmation()},
		Policy: func(_ context.Context, r toolsy.TypedPolicyRequest[subject, scope, args]) toolsy.Decision {
			if r.Context.Subject.ID != "operator" {
				return toolsy.DenyDecision("operator required")
			}
			return toolsy.AllowDecision()
		},
		Handler: func(_ context.Context, _ toolsy.TypedCallContext[subject, scope], _ *toolsy.RunEnv, a toolsy.ValidatedArgs[args]) (toolsy.ToolResult[args, string], error) {
			r := toolsy.NewToolResult[args, string](a.Value)
			r.Effects = []string{"recorded"}
			return r, nil
		},
	})
	if err != nil {
		return err
	}
	reg, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(tool).Build()
	if err != nil {
		return err
	}
	view, err := reg.View(toolsy.RegistryViewSpec{ToolNames: []string{recordTool}, Reason: "demo", Owner: "host"})
	if err != nil {
		return err
	}
	d, err := recipe.New[subject, scope, activity](
		view,
		func(_ context.Context, e []any) error { fmt.Println("fresh effects:", e); return nil },
	)
	if err != nil {
		return err
	}
	r := recipe.Request[subject, scope, activity]{
		CallID:                "provider-first",
		OperationID:           "intent-one",
		AttemptID:             "attempt-one",
		ActivityIdentity:      activity{Key: "activity-one"},
		Tool:                  recordTool,
		Subject:               subject{ID: "operator"},
		Scope:                 scope{ID: "workspace"},
		Args:                  json.RawMessage(`{"id":9007199254740993}`),
		PolicyFingerprint:     "operator-acl",
		DependencyFingerprint: "record-target",
	}
	pending, err := d.OneShot(ctx, r)
	if err != nil {
		return err
	}
	fmt.Println("host decision:", pending.Decision)
	// This is a trusted local operator action, not remote authentication.
	var challenge *toolsy.PendingApprovalError
	if !errors.As(pending.Err, &challenge) {
		return fmt.Errorf("expected approval: %w", pending.Err)
	}
	now := time.Now()
	if err = store.PutGrant(
		ctx,
		toolsy.ApprovalGrant{
			ID:        "local-grant",
			Issuer:    "host",
			Binding:   challenge.Challenge.Binding,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Minute),
		},
	); err != nil {
		return err
	}
	r.CallID, r.AttemptID, r.GrantID = "provider-resume", "attempt-two", "local-grant"
	result, err := d.OneShot(ctx, r)
	if err != nil {
		return err
	}
	if result.Decision != "continue" {
		return errors.Join(fmt.Errorf("resume: %s", result.Decision), result.Err)
	}
	wire, err := json.Marshal(result.Model)
	if err != nil {
		return err
	}
	fmt.Println(string(wire))
	r.CallID, r.AttemptID = "provider-redelivery", "attempt-three"
	replay, err := d.OneShot(ctx, r)
	if err != nil {
		return err
	}
	wire, err = json.Marshal(replay.Model)
	if err != nil {
		return err
	}
	fmt.Println(string(wire))
	return nil
}
