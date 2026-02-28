// Package main demonstrates a simple calculator tool with toolsy.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

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

	add, err := toolsy.NewTool("add", "Add two numbers", func(_ context.Context, args CalcArgs) (CalcResult, error) {
		return CalcResult{Sum: args.A + args.B}, nil
	})
	if err != nil {
		log.Fatalf("NewTool: %v", err)
	}

	reg := toolsy.NewRegistry(toolsy.WithDefaultTimeout(5 * time.Second))
	reg.Register(add)

	call := toolsy.ToolCall{
		ID:       "1",
		ToolName: "add",
		Args:     []byte(`{"a": 3, "b": 5}`),
	}
	var result []byte
	if err := reg.Execute(context.Background(), call, func(chunk []byte) error {
		result = chunk
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
