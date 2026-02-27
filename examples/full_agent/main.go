// Package main demonstrates multiple tools, ExecuteBatch, and partial success with toolsy.
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
	// Define tools
	type AddIn struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type AddOut struct {
		Sum int `json:"sum"`
	}
	add, err := toolsy.NewTool("add", "Add two integers", func(_ context.Context, in AddIn) (AddOut, error) {
		return AddOut{Sum: in.A + in.B}, nil
	})
	if err != nil {
		log.Fatalf("NewTool add: %v", err)
	}

	type MulIn struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type MulOut struct {
		Product int `json:"product"`
	}
	mul, err := toolsy.NewTool("mul", "Multiply two integers", func(_ context.Context, in MulIn) (MulOut, error) {
		return MulOut{Product: in.A * in.B}, nil
	})
	if err != nil {
		log.Fatalf("NewTool mul: %v", err)
	}

	reg := toolsy.NewRegistry(
		toolsy.WithDefaultTimeout(5*time.Second),
		toolsy.WithMaxConcurrency(4),
	)
	reg.Register(add)
	reg.Register(mul)

	// ExecuteBatch: run multiple calls in parallel (Partial Success — each result is independent)
	calls := []toolsy.ToolCall{
		{ID: "1", ToolName: "add", Args: []byte(`{"a": 1, "b": 2}`)},
		{ID: "2", ToolName: "mul", Args: []byte(`{"a": 3, "b": 4}`)},
		{ID: "3", ToolName: "add", Args: []byte(`{"a": 10, "b": 20}`)},
	}
	results := reg.ExecuteBatch(context.Background(), calls)

	for i, res := range results {
		if res.Error != nil {
			_, _ = fmt.Fprintf(os.Stderr, "call %s (%s): %v\n", res.CallID, res.ToolName, res.Error)
			// Self-correction: LLM can retry with corrected args when ClientError (e.g. validation)
			if toolsy.IsClientError(res.Error) {
				_, _ = fmt.Fprintln(os.Stderr, "  -> client error, LLM may retry with fixed input")
			}
			continue
		}
		switch res.ToolName {
		case "add":
			var out AddOut
			if err := json.Unmarshal(res.Result, &out); err != nil {
				log.Printf("unmarshal add result: %v", err)
				continue
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] add: sum=%d\n", i, out.Sum)
		case "mul":
			var out MulOut
			if err := json.Unmarshal(res.Result, &out); err != nil {
				log.Printf("unmarshal mul result: %v", err)
				continue
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] mul: product=%d\n", i, out.Product)
		}
	}
}
