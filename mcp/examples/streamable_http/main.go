package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/skosovsky/toolsy/mcp"
)

func main() {
	endpoint := os.Getenv("MCP_ENDPOINT")
	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "MCP_ENDPOINT is required")
		os.Exit(2)
	}
	token := os.Getenv("MCP_TOKEN")
	transport := mcp.NewStreamableHTTPTransport(
		endpoint,
		mcp.WithStreamableHTTPRequestDecorator(func(request *http.Request) error {
			if token != "" {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			return nil
		}),
	)
	client, err := mcp.Connect(context.Background(), transport)
	if err != nil {
		panic(err)
	}
	defer client.Close()
	fmt.Printf(
		"connected to %s %s with MCP %s\n",
		client.ServerInfo().ServerInfo.Name,
		client.ServerInfo().ServerInfo.Version,
		mcp.ProtocolVersion,
	)
}
