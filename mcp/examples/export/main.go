// Example export exposes one scoped local tool through SDK Server.AddTool.
// No client import adapter is required.
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

const echoName = "echo"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	local, err := toolsy.NewProxyTool(echoName, "Echo JSON", []byte(`{"type":"object"}`),
		func(_ context.Context, _ *toolsy.RunEnv, args []byte, yield func(toolsy.Chunk) error) error {
			return yield(toolsy.Chunk{Event: toolsy.EventResult, Data: args, MimeType: toolsy.MimeTypeJSON})
		})
	if err != nil {
		return err
	}
	registry, err := toolsy.NewRegistry(local)
	if err != nil {
		return err
	}
	view, err := registry.View(toolsy.RegistryViewSpec{ToolNames: []string{echoName}, Owner: "fixture-host"})
	if err != nil {
		return err
	}
	server, err := bridge.Export(
		view,
		bridge.ExportConfig{
			Names:          []string{echoName},
			Implementation: &sdk.Implementation{Name: "export", Version: "1"},
			Resolve: func(context.Context, sdk.Request) (toolsy.CallContext, error) {
				return toolsy.NewCallContext("fixture-user", "fixture-scope"), nil // Host-owned authenticated identity.
			},
		},
	)
	if err != nil {
		return err
	}
	client := sdk.NewClient(
		&sdk.Implementation{Name: "consumer", Version: "1"},
		&sdk.ClientOptions{MultiRoundTrip: &sdk.MultiRoundTripOptions{Disabled: true}},
	)
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
	result, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: echoName, Arguments: json.RawMessage(`{"n":7}`)})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}
