package starlark

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

func TestRunPrintsToStdout(t *testing.T) {
	sb := New()
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "hello", res.Stdout)
}

func TestRunReadsInMemoryFiles(t *testing.T) {
	sb := New()
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("data.txt"))`,
		Files:    map[string][]byte{"data.txt": []byte("world")},
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "world", res.Stdout)
}

func TestRunNormalizesFileLookups(t *testing.T) {
	sb := New()
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("dir/../data.txt"))`,
		Files:    map[string][]byte{"data.txt": []byte("world")},
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "world", res.Stdout)
}

func TestRunExposesEnv(t *testing.T) {
	sb := New()
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(env["NAME"])`,
		Env:      map[string]string{"NAME": "toolsy"},
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "toolsy", res.Stdout)
}

func TestRunReturnsScriptErrorsInStderr(t *testing.T) {
	sb := New()
	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print(fs.read("missing.txt"))`,
		Timeout:  time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.ExitCode)
	require.Contains(t, res.Stderr, "file not found")
}

func TestRunReturnsTimeout(t *testing.T) {
	sb := New()
	_, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code: `def run():
    for i in range(1000000000):
        pass
run()`,
		Timeout: 5 * time.Millisecond,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunRejectsInvalidInputFilePaths(t *testing.T) {
	sb := New()
	_, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
		Files:    map[string][]byte{"../secret.txt": []byte("nope")},
		Timeout:  time.Second,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestRunRejectsCollisionsAfterNormalization(t *testing.T) {
	sb := New()
	_, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "starlark",
		Code:     `print("hello")`,
		Files: map[string][]byte{
			"data.txt":        []byte("one"),
			"dir/../data.txt": []byte("two"),
		},
		Timeout: time.Second,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestExecToolSchemaExposesOnlyStarlark(t *testing.T) {
	sb := New()
	tool, err := exectool.New(sb, exectool.WithTimeout(time.Second))
	require.NoError(t, err)

	params := tool.Parameters()
	props := params["properties"].(map[string]any)
	language := props["language"].(map[string]any)
	require.Equal(t, []any{"starlark"}, language["enum"])
}

func TestRunRejectsPythonAlias(t *testing.T) {
	sb := New()
	_, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     `print("x")`,
		Timeout:  time.Second,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb := New()
	_, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "bash",
		Code:     `print("x")`,
		Timeout:  time.Second,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}
