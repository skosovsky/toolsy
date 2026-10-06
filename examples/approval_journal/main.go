// approval_journal is a standalone trusted-local-host example, not an agent loop.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/adapters/execution/filejournal"
)

const appendNoteTool = "append_note"

type operator struct{ ID string }
type workspace struct{ ID string }
type writeArgs struct {
	Note string `json:"note"`
}
type config struct {
	Directory, OperationID, Note string
	Approve                      bool
}

func main() {
	var cfg config
	flag.StringVar(&cfg.Directory, "directory", "", "absolute existing trusted local directory")
	flag.StringVar(&cfg.OperationID, "operation", "", "stable host intent ID; reuse for repeated delivery")
	flag.StringVar(&cfg.Note, "note", "example", "bounded note to append")
	flag.BoolVar(&cfg.Approve, "approve", false, "trusted local operator approval; never model input")
	flag.Parse()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	if cfg.OperationID == "" || len(cfg.OperationID) > 256 || len(cfg.Note) > 1024 {
		return errors.New("bounded operation ID and note required")
	}
	store, err := filejournal.Open(filepath.Join(cfg.Directory, "operations.json"), 0)
	if err != nil {
		return err
	}
	receiptPath := filepath.Join(cfg.Directory, "effects.jsonl")
	tool, err := newReceiptTool(receiptPath)
	if err != nil {
		return err
	}
	grantID := ""
	profile, err := toolsy.NewOperationProfile(
		toolsy.OperationProfileConfig{
			Store: store,
			Prepare: func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
				typed, contextErr := toolsy.TypedContext[operator, workspace](call.Context)
				if contextErr != nil {
					return toolsy.OperationIntent{}, contextErr
				}
				return toolsy.OperationIntent{
					Namespace:         "local-notes",
					Subject:           typed.Subject.ID,
					Scope:             typed.Scope.ID,
					OperationID:       cfg.OperationID,
					AttemptID:         call.Input.CallID,
					GrantID:           grantID,
					PolicyFingerprint: "local-operator-acl",
					CanonicalDigest:   receiptPath,
					CanonicalRules:    "typed JSON and trusted receipt target",
					DisplayJSON:       call.Input.ArgsJSON,
				}, nil
			},
			Codec:    toolsy.JSONResultCodec[string, string]{},
			Issuer:   "local-host",
			Clock:    time.Now,
			Lease:    time.Minute,
			MaxBytes: 0,
		},
	)
	if err != nil {
		return err
	}
	reg, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(tool).Build()
	if err != nil {
		return err
	}
	session, err := toolsy.NewSession(
		reg,
		toolsy.WithMaxCalls(2),
		toolsy.WithRunPolicy(toolsy.RunPolicy{AllowedTools: []string{appendNoteTool}}),
	)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(writeArgs{Note: cfg.Note})
	if err != nil {
		return err
	}
	call := toolsy.ToolCall{
		ToolName:    appendNoteTool,
		CallContext: toolsy.NewCallContext(operator{ID: "local-operator"}, workspace{ID: "local-workspace"}),
		Input:       toolsy.ToolInput{CallID: rand.Text(), ArgsJSON: raw},
	}
	yield := func(c toolsy.Chunk) error {
		return printResult(out, c)
	}
	err = session.Execute(ctx, call, yield)
	var pending *toolsy.PendingApprovalError
	if !errors.As(err, &pending) {
		return err
	} // completed replays; unknown/conflict never gets a new grant.
	if _, writeErr := fmt.Fprintf(out, "pending action=%s\n", pending.Challenge.DisplayJSON); writeErr != nil {
		return writeErr
	}
	if !cfg.Approve {
		return nil
	} // host persists continuation, not a successful execution.
	// This CLI flag is a deliberate trusted local operator action. A remote host
	// must authenticate the approver and bind its UI response to this exact challenge.
	now := time.Now()
	grantID = rand.Text()
	if err := store.PutGrant(
		ctx,
		toolsy.ApprovalGrant{
			ID:        grantID,
			Issuer:    "local-host",
			Binding:   pending.Challenge.Binding,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Minute),
		},
	); err != nil {
		return err
	}
	call.Input.CallID = rand.Text()
	return session.Execute(ctx, call, yield)
}

func newReceiptTool(receiptPath string) (toolsy.Tool, error) {
	return toolsy.NewTypedTool(toolsy.TypedToolSpec[operator, workspace, writeArgs, string, string]{
		Name:        appendNoteTool,
		Description: "Append a note to a host-owned local receipt",
		Options:     []toolsy.ToolOption{toolsy.WithRequiresConfirmation()},
		Policy: func(_ context.Context, req toolsy.TypedPolicyRequest[operator, workspace, writeArgs]) toolsy.Decision {
			if req.Context.Subject.ID != "local-operator" || req.Context.Scope.ID != "local-workspace" {
				return toolsy.DenyDecision("host identity required")
			}
			return toolsy.AllowDecision()
		},
		Handler: func(_ context.Context, _ toolsy.TypedCallContext[operator, workspace], _ *toolsy.RunEnv, args toolsy.ValidatedArgs[writeArgs]) (toolsy.ToolResult[string, string], error) {
			if err := appendReceipt(receiptPath, args.Value); err != nil {
				return toolsy.ToolResult[string, string]{}, err
			}
			result := toolsy.NewToolResult[string, string]("recorded")
			result.Audience = toolsy.AudienceUser
			return result, nil
		},
	})
}

func printResult(out io.Writer, c toolsy.Chunk) error {
	if c.Event != toolsy.EventResult {
		return nil
	}
	_, err := fmt.Fprintf(out, "result=%s replay=%v\n", c.Data, c.ToolEnvelope().Metadata[toolsy.ReplaySourceMetadata])
	return err
}

func appendReceipt(path string, args writeArgs) (err error) {
	// The host controls this directory/constant filename; model input contains only Note.
	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o600,
	) // #nosec G703 -- trusted host-owned receipt path, never model input.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := json.NewEncoder(file).Encode(args); err != nil {
		return err
	}
	return file.Sync()
}
