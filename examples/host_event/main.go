// Host-owned routing of neutral control data. Core never dispatches a UI action.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

func main() {
	tool, err := toolsy.NewStreamTool(
		"request_panel",
		"request panel",
		func(_ context.Context, _ *toolsy.RunEnv, _ struct{}, yield func(toolsy.Chunk) error) error {
			return toolsy.YieldControl(
				yield,
				&toolsy.HostEventSignal{Name: "ui.open_panel", PayloadJSON: []byte(`{"id":"settings"}`)},
			)
		},
		toolsy.WithIndependentStream(),
	)
	if err != nil {
		log.Fatal(err)
	}
	registry, err := toolsy.NewRegistry(tool)
	if err != nil {
		log.Fatal(err)
	}
	err = registry.Execute(
		context.Background(),
		toolsy.ToolCall{ToolName: "request_panel", Input: toolsy.ToolInput{ArgsJSON: []byte(`{}`)}},
		func(chunk toolsy.Chunk) error {
			event, ok := chunk.Control.(*toolsy.HostEventSignal)
			if !ok {
				return errors.New("host: expected named event")
			}
			return dispatchHostEvent(event)
		},
	)
	if !errors.Is(err, toolsy.ErrHostEvent) {
		log.Fatalf("control delivery: %v", err)
	}
	fmt.Println("host received event; scheduling remains host-owned")
}

// A host allowlist owns interpretation. Payload is data, never executable code.
// A real shell checks its own authorization and invokes its UI API here.
func dispatchHostEvent(event *toolsy.HostEventSignal) error {
	if event.Name != "ui.open_panel" {
		return errors.New("host: event is not allowed")
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
		return err
	}
	if payload.ID != "settings" {
		return errors.New("host: panel is not allowed")
	}
	fmt.Printf("host routed panel request: %s\n", payload.ID)
	return nil
}
