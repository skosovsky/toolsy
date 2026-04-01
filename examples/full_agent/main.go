// Package main demonstrates multiple tools, ExecuteBatchStream, and streaming with toolsy.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/skosovsky/toolsy"
)

func main() {
	add, mul, err := buildTools()
	if err != nil {
		log.Fatalf("build tools: %v", err)
	}
	reg, err := toolsy.NewRegistryBuilder().
		Use(toolsy.WithLogging(slog.Default())).Add(add, mul).Build()
	if err != nil {
		log.Fatalf("build registry: %v", err)
	}

	if err := runBatchStream(reg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "batch error: %v\n", err)
		if toolsy.IsClientError(err) {
			_, _ = fmt.Fprintln(os.Stderr, "  -> client error, LLM may retry with fixed input")
		}
	}
}

func buildTools() (toolsy.Tool, toolsy.Tool, error) {
	type AddIn struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type AddOut struct {
		Sum int `json:"sum"`
	}
	add, err := toolsy.NewTool(
		"add",
		"Add two integers",
		func(_ context.Context, _ toolsy.RunContext, in AddIn) (AddOut, error) {
			return AddOut{Sum: in.A + in.B}, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}

	type MulIn struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type MulOut struct {
		Product int `json:"product"`
	}
	mul, err := toolsy.NewTool(
		"mul",
		"Multiply two integers",
		func(_ context.Context, _ toolsy.RunContext, in MulIn) (MulOut, error) {
			return MulOut{Product: in.A * in.B}, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return add, mul, nil
}

func runBatchStream(reg *toolsy.Registry) error {
	type AddOut struct {
		Sum int `json:"sum"`
	}
	type MulOut struct {
		Product int `json:"product"`
	}
	calls := []toolsy.ToolCall{
		{ToolName: "add", Input: toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{"a": 1, "b": 2}`)}},
		{ToolName: "mul", Input: toolsy.ToolInput{CallID: "2", ArgsJSON: []byte(`{"a": 3, "b": 4}`)}},
		{ToolName: "add", Input: toolsy.ToolInput{CallID: "3", ArgsJSON: []byte(`{"a": 10, "b": 20}`)}},
	}
	var idx int
	return reg.ExecuteBatchStream(context.Background(), calls, func(c toolsy.Chunk) error {
		if c.IsError {
			log.Printf("tool error [%s]: %s", c.ToolName, c.Data)
			return nil
		}
		switch c.ToolName {
		case "add":
			var out AddOut
			if err := json.Unmarshal(c.Data, &out); err != nil {
				log.Printf("decode add result: %v", err)
				return nil
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] add: sum=%d\n", idx, out.Sum)
		case "mul":
			var out MulOut
			if err := json.Unmarshal(c.Data, &out); err != nil {
				log.Printf("decode mul result: %v", err)
				return nil
			}
			_, _ = fmt.Fprintf(os.Stdout, "result[%d] mul: product=%d\n", idx, out.Product)
		}
		idx++
		return nil
	})
}
