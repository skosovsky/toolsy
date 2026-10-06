// Example host routing after a typed post-handler contract failure.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

func main() {
	effects := 0
	tool, err := toolsy.NewTypedTool(toolsy.TypedToolSpec[toolsy.NoSubject, toolsy.NoScope, struct{}, string, struct{}]{
		Name:        "write",
		Description: "Write",
		Handler: func(context.Context, toolsy.TypedCallContext[toolsy.NoSubject, toolsy.NoScope], *toolsy.RunEnv, toolsy.ValidatedArgs[struct{}]) (toolsy.ToolResult[string, struct{}], error) {
			effects++ // Represents an external write already made by the handler.
			return toolsy.NewToolResult[string, struct{}]("written"), nil
		},
		ResultValidator: func(string) error { return errors.New("result contract rejected") },
	})
	if err != nil {
		panic(err)
	}
	execute := func() error {
		return tool.Execute(
			context.Background(),
			toolsy.NewRunEnv(nil),
			toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
			func(toolsy.Chunk) error { return nil },
		)
	}
	err = execute()
	if te, ok := toolsy.AsToolError(err); ok && toolsy.ClientCorrectable(te.Code) {
		// A real host asks for corrected arguments before a new authorized call.
		fmt.Println("argument correction required")
		return
	}
	if contract, ok := errors.AsType[*toolsy.ResultContractError](err); ok {
		// Keep the operation reference and reconcile the external write. No retry.
		fmt.Printf("reconcile: phase=%s effects=%d\n", contract.Kind, effects)
		return
	}
	if err != nil {
		panic(err)
	}
}
