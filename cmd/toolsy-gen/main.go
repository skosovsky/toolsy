// Command toolsy-gen generates Go DTOs, handlers, and tool factories from YAML/JSON manifests.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/skosovsky/toolsy/internal/toolsygen"
)

func main() {
	args := []string{}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}
	os.Exit(run(args, os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("toolsy-gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stdout, "Usage: toolsy-gen [path ...]")
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintln(stdout, "Generate Go DTOs, handlers, and tool factories from YAML/JSON tool manifests.")
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintln(stdout, "If no paths are provided, the current directory is scanned recursively.")
	}

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.Usage()
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "toolsy-gen: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result, err := toolsygen.Generate(ctx, toolsygen.Config{Inputs: fs.Args()})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "toolsy-gen: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "generated %d file(s)\n", len(result.Files))
	for _, path := range result.Files {
		_, _ = fmt.Fprintln(stdout, path)
	}
	return 0
}
