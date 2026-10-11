package toolsy_test

import (
	"context"
	"fmt"

	"github.com/skosovsky/toolsy"
)

func ExampleAdmitInput() {
	// Arrange: a host owns the batch and its trusted call context.
	ctx := context.Background()
	handlers := 0
	tool, err := toolsy.NewDynamicToolFromSpec(toolsy.DynamicToolSpec{
		Name: "write",
		Schema: toolsy.MapSchemaProvider{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []string{"text"},
		},
		Handler: func(_ context.Context, _ *toolsy.RunEnv, _ map[string]any, yield func(toolsy.Chunk) error) error {
			handlers++
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`"ok"`), MimeType: toolsy.MimeTypeJSON})
		},
	})
	if err != nil {
		panic(err)
	}
	reg, err := toolsy.NewRegistryBuilder().Add(tool).Build()
	if err != nil {
		panic(err)
	}
	runBatch := func(inputs []toolsy.ToolInput) error {
		for _, input := range inputs {
			if _, admitErr := toolsy.AdmitInput(ctx, tool, toolsy.CallContext{}, input); admitErr != nil {
				return admitErr
			}
		}
		for _, input := range inputs {
			if executeErr := reg.Execute(
				ctx,
				toolsy.ToolCall{ToolName: "write", Input: input},
				func(toolsy.Chunk) error { return nil },
			); executeErr != nil {
				return executeErr
			}
		}
		return nil
	}
	// Act: no handler runs when a later slot fails input validation.
	err = runBatch([]toolsy.ToolInput{{ArgsJSON: []byte(`{"text":"first"}`)}, {ArgsJSON: []byte(`{}`)}})
	// Assert.
	fmt.Println("rejected:", err != nil, "handlers:", handlers)
	// Act: dispatch always repeats binding and authorization for the current input.
	err = runBatch([]toolsy.ToolInput{{ArgsJSON: []byte(`{"text":"first"}`)}, {ArgsJSON: []byte(`{"text":"second"}`)}})
	// Assert.
	fmt.Println("accepted:", err == nil, "handlers:", handlers)
	// Output:
	// rejected: true handlers: 0
	// accepted: true handlers: 2
}
