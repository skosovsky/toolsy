// Package main demonstrates schema-validated streaming export without an agent.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

func main() {
	tool, err := toolsy.NewStreamTool(
		"export",
		"Export summary",
		func(_ context.Context, _ *toolsy.RunEnv, _ struct{}, yield func(toolsy.Chunk) error) error {
			if err := yield(
				toolsy.Chunk{Event: toolsy.EventProgress, Data: []byte("exporting"), MimeType: toolsy.MimeTypeText},
			); err != nil {
				return err
			}
			return yield(
				toolsy.Chunk{Event: toolsy.EventResult, Data: []byte(`{"rows":3}`), MimeType: toolsy.MimeTypeJSON},
			)
		},
		toolsy.WithTerminalStream(0),
		toolsy.WithOutputSchema(
			map[string]any{
				"type":       "object",
				"properties": map[string]any{"rows": map[string]any{"type": "integer"}},
				"required":   []string{"rows"},
			},
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	err = tool.Execute(
		context.Background(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{}`)},
		func(c toolsy.Chunk) error { fmt.Printf("%s: %s\n", c.Event, c.Data); return nil },
	)
	if err != nil {
		log.Fatal(err)
	}
}
