package starlark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
	"github.com/skosovsky/toolsy/textprocessor"
)

func TestRunPrintsToStdout(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "hello\n", res.Stdout)
}

func TestRunReadsInMemoryFiles(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("data.txt"))`,
		Files:    map[string][]byte{"data.txt": []byte("world")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "world\n", res.Stdout)
}

func TestRunNormalizesFileLookups(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("dir/../data.txt"))`,
		Files:    map[string][]byte{"data.txt": []byte("world")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "world\n", res.Stdout)
}

func TestRunExposesEnv(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(env["NAME"])`,
		Env:      map[string]string{"NAME": "toolsy"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "toolsy\n", res.Stdout)
}

func TestRunReturnsScriptErrorsInStderr(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("missing.txt"))`,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.ExitCode)
	require.Contains(t, res.Stderr, "file not found")
}

func TestRunReturnsTimeout(t *testing.T) {
	sb, err := New(Config{MaxExecutionSteps: 1 << 62})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "starlark",
		Code: `def run():
    for i in range(1000000000):
        pass
run()`,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunRejectsInvalidInputFilePaths(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
		Files:    map[string][]byte{"../secret.txt": []byte("nope")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestRunRejectsCollisionsAfterNormalization(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
		Files: map[string][]byte{
			"data.txt":        []byte("one"),
			"dir/../data.txt": []byte("two"),
		},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestExecToolSchemaExposesOnlyStarlark(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	tool, err := exectool.New(sb)
	require.NoError(t, err)

	params := tool.Manifest().Parameters
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"starlark"}, language["enum"])
}

func TestRunRejectsPythonAlias(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     `print("x")`,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "bash",
		Code:     `print("x")`,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestRunRejectsOversizedEvalStderr(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	huge := strings.Repeat("e", sandboxfs.DefaultMaxSandboxOutputBytes+1)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     fmt.Sprintf("fail(%q)", huge),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestRunRejectsStdoutExceedingCap(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	big := strings.Repeat("x", sandboxfs.DefaultMaxSandboxOutputBytes+1)
	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     fmt.Sprintf(`print(%q)`, big),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
}

func TestRunRejectsFileReadExceedingCap(t *testing.T) {
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	big := make([]byte, sandboxfs.DefaultMaxSandboxFileReadBytes+1)
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("big.txt"))`,
		Files:    map[string][]byte{"big.txt": big},
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.ExitCode)
	require.Contains(t, res.Stderr, "exceeds")
	require.Contains(t, res.Stderr, "read operation exceeded configured byte limit")
}

func TestNewRejectsUnlimitedExecution(t *testing.T) {
	// Arrange.
	config := Config{}
	// Act.
	sb, err := New(config)
	// Assert.
	require.Nil(t, sb)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

// Each workload is isolated in a subprocess: a broken interpreter cancellation
// policy must not hang the parent test suite.
func TestRunBoundsComputation(t *testing.T) {
	mode := os.Getenv("TOOLSY_STARLARK_BOUND_TEST")
	if mode == "" {
		for _, childMode := range []string{"steps", "cancel"} {
			t.Run(childMode, func(t *testing.T) {
				// Arrange.
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunBoundsComputation$", "-test.timeout=8s")
				cmd.Env = append(os.Environ(), "TOOLSY_STARLARK_BOUND_TEST="+childMode)
				// Act.
				output, err := cmd.CombinedOutput()
				// Assert.
				require.NoError(t, ctx.Err(), string(output))
				require.NoError(t, err, string(output))
			})
		}
		return
	}
	// Arrange. This workload would require billions of years without a limit;
	// default Starlark forbids literal infinite while loops and recursion.
	config := Config{MaxExecutionSteps: 1000}
	ctx := context.Background()
	if mode == "cancel" {
		config.MaxExecutionSteps = 1 << 62
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
	}
	sb, err := New(config)
	require.NoError(t, err)
	started := time.Now()
	// Act.
	_, err = sb.Run(ctx, exectool.RunRequest{Language: "starlark", Code: `def run():
    for i in range(1 << 62):
        pass
run()`})
	// Assert.
	require.Less(t, time.Since(started), time.Second)
	if mode == "steps" {
		require.ErrorIs(t, err, ErrStepLimit)
		require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	} else {
		require.ErrorIs(t, err, exectool.ErrTimeout)
		require.NotErrorIs(t, err, ErrStepLimit)
	}
}

func TestRunAlreadyCanceledDoesNotEvaluate(t *testing.T) {
	// Arrange.
	sb, err := New(DefaultConfig())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	_, err = sb.Run(ctx, exectool.RunRequest{Language: "starlark", Code: `fail("guest error")`})
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunPreservesExactPrintedBytesForEveryGuestExit(t *testing.T) {
	for _, tc := range []struct {
		name, code, want string
		exit             int
	}{
		{"success", `print("hello")`, "hello\n", 0},
		{"failure", `print("hello"); fail("guest")`, "hello\n", 1},
		{"success_blank_lines", `print("hello\n"); print()`, "hello\n\n\n", 0},
		{"failure_blank_lines", `print("hello\n"); print(); fail("guest")`, "hello\n\n\n", 1},
		{"success_empty", `value = 1`, "", 0},
		{"failure_empty", `fail("guest")`, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			sb, err := New(DefaultConfig())
			require.NoError(t, err)
			// Act.
			result, err := sb.Run(t.Context(), exectool.RunRequest{Language: "starlark", Code: tc.code})
			// Assert.
			require.NoError(t, err)
			require.Equal(t, tc.exit, result.ExitCode)
			require.Equal(t, tc.want, result.Stdout)
		})
	}
}
