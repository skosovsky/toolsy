package toolsy_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/toolsy"
)

func ExampleNewAuthorizerPolicy() {
	// Capture the host port rather than resolving authorization through RunEnv.
	authorizer := toolsy.AuthorizerFunc(func(_ context.Context, request toolsy.PolicyRequest) error {
		if request.CallContext.Subject != "alice" {
			return errors.New("caller is not authorized")
		}
		return nil
	})
	policy, err := toolsy.NewAuthorizerPolicy(authorizer)
	if err != nil {
		panic(err)
	}
	echo, err := toolsy.NewTool(
		"echo",
		"Echo",
		func(_ context.Context, _ *toolsy.RunEnv, input struct{ Text string }) (string, error) {
			return input.Text, nil
		},
	)
	if err != nil {
		panic(err)
	}
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithPolicy("host-auth-v1", policy)).Add(echo).Build()
	if err != nil {
		panic(err)
	}
	err = registry.Execute(context.Background(), toolsy.ToolCall{
		ToolName: "echo", Input: toolsy.ToolInput{ArgsJSON: []byte(`{"Text":"hello"}`)},
		CallContext: toolsy.NewCallContext("alice", "tenant"),
	}, func(chunk toolsy.Chunk) error { fmt.Println(string(chunk.Data)); return nil })
	if err != nil {
		panic(err)
	}
	// Output: "hello"
}
