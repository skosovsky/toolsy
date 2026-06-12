// Package main demonstrates a simple calculator tool with Session.RunCall.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/skosovsky/toolsy"
)

func main() {
	type CalcArgs struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type CalcResult struct {
		Sum int `json:"sum"`
	}

	add, err := toolsy.NewTool(
		"add",
		"Add two numbers",
		func(_ context.Context, _ *toolsy.RunEnv, args CalcArgs) (CalcResult, error) {
			return CalcResult{Sum: args.A + args.B}, nil
		},
	)
	if err != nil {
		log.Fatalf("NewTool: %v", err)
	}

	type SubResult struct {
		Diff int `json:"diff"`
	}
	sub, err := toolsy.NewTool(
		"sub",
		"Subtract b from a",
		func(_ context.Context, _ *toolsy.RunEnv, args CalcArgs) (SubResult, error) {
			return SubResult{Diff: args.A - args.B}, nil
		},
	)
	if err != nil {
		log.Fatalf("NewTool sub: %v", err)
	}

	reg, err := toolsy.NewRegistryBuilder().Add(add, sub).Build()
	if err != nil {
		log.Fatalf("registry build: %v", err)
	}
	sess, err := toolsy.NewSession(reg)
	if err != nil {
		log.Fatalf("session: %v", err)
	}
	_ = add.Manifest().Parameters

	outcome, err := sess.RunCall(context.Background(), toolsy.ToolCall{
		ToolName: "add",
		Input: toolsy.ToolInput{
			CallID:   "1",
			ArgsJSON: []byte(`{"a": 3, "b": 5}`),
		},
		Env: toolsy.NewRunEnv(sess),
	})
	if err != nil {
		log.Fatalf("run call: %v", err)
	}
	if outcome.ExecutionError != nil {
		log.Fatalf("execution error: %v", outcome.ExecutionError)
	}
	out, err := toolsy.DecodeOutcomeAs[CalcResult](outcome)
	if err != nil {
		log.Fatalf("decode outcome: %v", err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "3 + 5 = %d\n", out.Sum)
}
