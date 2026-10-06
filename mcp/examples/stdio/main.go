package main

import (
	"context"
	"fmt"
	"os"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/mcp"
)

func main() {
	command := os.Getenv("MCP_SERVER_COMMAND")
	if command == "" {
		fmt.Fprintln(os.Stderr, "MCP_SERVER_COMMAND is required")
		os.Exit(2)
	}

	ctx := context.Background()
	transport := mcp.NewStdioTransport(command, os.Args[1:])
	client, err := mcp.Connect(ctx, transport)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	builder := toolsy.NewRegistryBuilder()
	for proxy, iterErr := range client.Discover(ctx) {
		if iterErr != nil {
			panic(iterErr)
		}
		builder.Add(proxy)
	}
	registry, err := builder.Build()
	if err != nil {
		panic(err)
	}
	fmt.Println("registered tools:", registry.ToolNames())
}
