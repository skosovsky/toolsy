package e2b_test

import (
	"context"
	"fmt"
	"io"

	"github.com/skosovsky/toolsy/adapters/sandbox/e2b"
	"github.com/skosovsky/toolsy/exectool"
)

// demoClient checks the public seam locally; it is not a cloud SDK implementation.
type demoClient struct{}
type demoSession struct{}

func (demoClient) CreateSandbox(context.Context) (e2b.Session, error) { return demoSession{}, nil }
func (demoSession) WriteFile(context.Context, string, []byte) error   { return nil }
func (demoSession) Kill(context.Context) error                        { return nil }

func (demoSession) StartAndWait(
	_ context.Context,
	command string,
	args []string,
	_ map[string]string,
	stdout, _ io.Writer,
) (e2b.CommandResult, error) {
	// A real client transports executable/argv exactly once and forwards writer errors.
	_, err := fmt.Fprintf(stdout, "%s %q", command, args)
	return e2b.CommandResult{ExitCode: 0}, err
}

func ExampleNew() {
	sb, err := e2b.New(demoClient{}, e2b.WithRuntime("custom", e2b.Runtime{
		Command:    "python",
		Args:       []string{"-u", "/workspace/dir/../main script.py", "$literal"},
		ScriptName: "dir/../main script.py",
	}))
	if err != nil {
		panic(err)
	}
	result, err := sb.Run(context.Background(), exectool.RunRequest{Language: "custom", Code: "print(1)"})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Stdout)
	// Output: python ["-u" "/workspace/main script.py" "$literal"]
}
