// Package main demonstrates a simple calculator tool with toolsy.
package main

import (
	"context"
	"encoding/json"
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
		func(_ context.Context, _ toolsy.RunContext, args CalcArgs) (CalcResult, error) {
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
		func(_ context.Context, _ toolsy.RunContext, args CalcArgs) (SubResult, error) {
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
	// Schema for LLM: pass add.Manifest().Parameters or sub.Manifest().Parameters to your provider (do not mutate)
	_ = add.Manifest().Parameters

	call := toolsy.ToolCall{
		ToolName: "add",
		Input: toolsy.ToolInput{
			CallID:   "1",
			ArgsJSON: []byte(`{"a": 3, "b": 5}`),
		},
	}
	var result []byte
	if err := reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		result = c.Data
		return nil
	}); err != nil {
		log.Fatalf("execute: %v", err)
	}
	var out CalcResult
	if err := json.Unmarshal(result, &out); err != nil {
		log.Fatalf("unmarshal: %v", err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "3 + 5 = %d\n", out.Sum)
}
