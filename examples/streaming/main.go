// Package main demonstrates NewStreamTool and chunk-by-chunk streaming with toolsy.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/skosovsky/toolsy"
)

func main() {
	type QueryArgs struct {
		Limit int `json:"limit" jsonschema:"Max results"`
	}
	tool, err := toolsy.NewStreamTool("stream_numbers", "Stream numbers 1..N", func(_ context.Context, q QueryArgs, yield func(toolsy.Chunk) error) error {
		for i := 1; i <= q.Limit; i++ {
			chunk, _ := json.Marshal(map[string]int{"n": i})
			if err := yield(toolsy.Chunk{Data: chunk}); err != nil {
				return err // e.g. ErrStreamAborted if client closed
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("NewStreamTool: %v", err)
	}

	reg := toolsy.NewRegistry(toolsy.WithDefaultTimeout(5 * time.Second))
	reg.Register(tool)

	call := toolsy.ToolCall{ID: "1", ToolName: "stream_numbers", Args: []byte(`{"limit": 3}`)}
	var count int
	err = reg.Execute(context.Background(), call, func(c toolsy.Chunk) error {
		count++
		var v map[string]int
		_ = json.Unmarshal(c.Data, &v)
		_, _ = fmt.Fprintf(os.Stdout, "chunk %d: n=%d\n", count, v["n"])
		return nil
	})
	if err != nil {
		log.Fatalf("execute: %v", err)
	}
	// chunk 1: n=1, chunk 2: n=2, chunk 3: n=3
}
