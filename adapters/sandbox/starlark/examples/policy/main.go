package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/adapters/sandbox/starlark"
	"github.com/skosovsky/toolsy/exectool"
)

const (
	hostSteps    uint64 = 1000
	callerBudget        = 2 * time.Second
)

func run(ctx context.Context, code string, out io.Writer) error {
	config := starlark.DefaultConfig()
	config.MaxExecutionSteps = hostSteps
	sandbox, err := starlark.New(config)
	if err != nil {
		return err
	}
	tool, err := exectool.New(sandbox, exectool.WithAllowedLanguages("starlark"))
	if err != nil {
		return err
	}
	input, err := json.Marshal(map[string]string{"language": "starlark", "code": code})
	if err != nil {
		return err
	}
	return tool.Execute(ctx, toolsy.NewRunEnv(nil), toolsy.ToolInput{ArgsJSON: input}, func(chunk toolsy.Chunk) error {
		_, err := fmt.Fprintln(out, string(chunk.Data))
		return err
	})
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), callerBudget)
	defer cancel()
	if err := run(ctx, `print("bounded")`, os.Stdout); err != nil {
		panic(err)
	}
}
