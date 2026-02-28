// Package main demonstrates multiple tools, ExecuteBatchStream, and streaming with toolsy.
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

	// ExecuteBatchStream: run multiple calls in parallel; yield receives Chunk (CallID, ToolName, Data)
	calls := []toolsy.ToolCall{
		{ID: "1", ToolName: "add", Args: []byte(`{"a": 1, "b": 2}`)},
		{ID: "2", ToolName: "mul", Args: []byte(`{"a": 3, "b": 4}`)},
		{ID: "3", ToolName: "add", Args: []byte(`{"a": 10, "b": 20}`)},
	}
	var idx int
	err = reg.ExecuteBatchStream(context.Background(), calls, func(c toolsy.Chunk) error {
		switch c.ToolName {
		case "add":
			var out AddOut
			if e := json.Unmarshal(c.Data, &out); e != nil {
				log.Printf("unmarshal add result: %v", e)
				return nil
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] add: sum=%d\n", idx, out.Sum)
		case "mul":
			var out MulOut
			if e := json.Unmarshal(c.Data, &out); e != nil {
				log.Printf("unmarshal mul result: %v", e)
				return nil
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] mul: product=%d\n", idx, out.Product)
		}
		idx++
		return nil
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "batch error: %v\n", err)
		if toolsy.IsClientError(err) {
			_, _ = fmt.Fprintln(os.Stderr, "  -> client error, LLM may retry with fixed input")
		}
	}
}
