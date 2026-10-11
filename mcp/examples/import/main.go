// Example import creates a host SDK connection and executes an imported tool
// through toolsy's registry. No export adapter is required.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/toolsy"
	bridge "github.com/skosovsky/toolsy/mcp"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	server := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&sdk.Tool{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{StructuredContent: req.Params.Arguments}, nil
		})
	hooks, options, err := bridge.ComposeClient(nil, bridge.ImportConfig{
		Classify: func(context.Context, *sdk.Tool) ([]toolsy.ToolOption, error) {
			return []toolsy.ToolOption{toolsy.WithReadOnly()}, nil // Trusted fixture only.
		},
	})
	if err != nil {
		return err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, options)
	if installErr := hooks.Install(client); installErr != nil {
		return installErr
	}
	a, b := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, b, nil)
	if err != nil {
		return err
	}
	defer ss.Close()
	cs, err := client.Connect(ctx, a, nil)
	if err != nil {
		return err
	}
	defer cs.Close()
	importer, err := hooks.Bind(cs, "host-fixture-scope")
	if err != nil {
		return err
	}
	defer importer.Close()
	snapshot, err := importer.Discover(ctx)
	if err != nil {
		return err
	}
	registry, err := toolsy.NewRegistry(snapshot.Tools...)
	if err != nil {
		return err
	}
	return registry.Execute(
		ctx,
		toolsy.ToolCall{ToolName: "echo", Input: toolsy.ToolInput{ArgsJSON: []byte(`{"n":7}`)}},
		func(chunk toolsy.Chunk) error {
			if chunk.Event == toolsy.EventResult {
				fmt.Println(string(chunk.Data))
			}
			return nil
		},
	)
}
