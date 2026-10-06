package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/skosovsky/toolsy/internal/release"
)

func main() {
	prepare := flag.Bool("prepare-only", false, "verify committed artifacts without publication")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: toolsy-release [-prepare-only] patch|break [module-list]")
		os.Exit(1)
	}
	modules := ""
	if len(args) == 2 {
		modules = args[1]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := release.Run(
		ctx,
		release.Config{
			Dir:         ".",
			Mode:        args[0],
			Modules:     modules,
			PrepareOnly: *prepare, FullChecks: true,
			Input:  os.Stdin,
			Output: os.Stdout,
		},
	); err != nil {
		stop()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stop()
}
