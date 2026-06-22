// Package main demonstrates RunCall, ToolOutcome, and DecodeOutcomeAs.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

type doubleArgs struct {
	N int `json:"n"`
}

type doubleResult struct {
	Double int `json:"double"`
}

func main() {
	tool := mustDoubleTool()
	reg := mustRegistry(tool)
	sess := mustSession(reg)

	call := toolsy.ToolCall{
		ToolName: "double",
		Input:    toolsy.ToolInput{CallID: "1", ArgsJSON: []byte(`{"n":21}`)},
		Env:      toolsy.NewRunEnv(sess),
	}
	fmt.Printf("double(21) = %d\n", runDouble(sess, call))

	call.Input.ArgsJSON = []byte(`{"n":-1}`)
	demoValidationFailure(sess, call)
}

func mustDoubleTool() toolsy.Tool {
	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[
		toolsy.NoSubject,
		toolsy.NoScope,
		doubleArgs,
		doubleResult,
		struct{},
	]{
		Name:        "double",
		Description: "Double an integer",
		ArgValidator: func(a doubleArgs) error {
			if a.N < 0 {
				return toolsy.NewValidationError("n must be non-negative")
			}
			return nil
		},
		Handler: func(
			_ context.Context,
			_ toolsy.TypedCallContext[toolsy.NoSubject, toolsy.NoScope],
			_ *toolsy.RunEnv,
			a toolsy.ValidatedArgs[doubleArgs],
		) (toolsy.ToolResult[doubleResult, struct{}], error) {
			return toolsy.NewToolResult[doubleResult, struct{}](doubleResult{Double: a.Value.N * 2}), nil
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return tool
}

func mustRegistry(tool toolsy.Tool) *toolsy.Registry {
	reg, err := toolsy.NewRegistryBuilder().Add(tool).Use(toolsy.WithErrorFormatter()).Build()
	if err != nil {
		log.Fatal(err)
	}
	return reg
}

func mustSession(reg *toolsy.Registry) *toolsy.Session {
	sess, err := toolsy.NewSession(reg)
	if err != nil {
		log.Fatal(err)
	}
	return sess
}

func runDouble(sess *toolsy.Session, call toolsy.ToolCall) int {
	outcome, err := sess.RunCall(context.Background(), call)
	if err != nil {
		log.Fatalf("infrastructure failure: %v", err)
	}
	if outcome.ExecutionError != nil {
		log.Fatalf("business failure: %v", outcome.ExecutionError)
	}
	decoded, err := toolsy.DecodeOutcomeAs[doubleResult](outcome)
	if err != nil {
		log.Fatal(err)
	}
	return decoded.Double
}

func demoValidationFailure(sess *toolsy.Session, call toolsy.ToolCall) {
	outcome, err := sess.RunCall(context.Background(), call)
	if err != nil {
		log.Fatalf("unexpected infrastructure failure: %v", err)
	}
	if outcome.ExecutionError == nil {
		log.Fatal("expected validation ExecutionError")
	}
	te, ok := toolsy.AsToolError(outcome.ExecutionError)
	if !ok || te.Code != toolsy.CodeValidationFailed {
		log.Fatalf("expected CodeValidationFailed, got %v", outcome.ExecutionError)
	}
	fmt.Println("validation error routed via outcome.ExecutionError (not err)")
}
