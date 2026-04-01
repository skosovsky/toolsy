package host

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

const helperScriptName = "main.txt"

func helperRuntime() Runtime {
	return Runtime{
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestHostHelperProcess", "--"},
		ScriptName: helperScriptName,
	}
}

func TestNewRequiresRuntime(t *testing.T) {
	_, err := New()
	require.Error(t, err)
}

func TestNewRejectsDuplicateLanguagesAfterTrimming(t *testing.T) {
	_, err := New(
		WithRuntime("python", Runtime{Command: "python3", ScriptName: "main.py"}),
		WithRuntime(" python ", Runtime{Command: "python3", ScriptName: "other.py"}),
	)
	require.Error(t, err)
}

func TestNewRejectsInvalidScriptName(t *testing.T) {
	_, err := New(WithRuntime("helper", Runtime{
		Command:    os.Args[0],
		ScriptName: "../main.txt",
	}))
	require.Error(t, err)
}

func TestNewCanonicalizesScriptName(t *testing.T) {
	sb, err := New(WithRuntime("helper", Runtime{
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestHostHelperProcess", "--"},
		ScriptName: `dir\main.txt`,
	}))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "read",
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"TOOLSY_ENV":             "present",
		},
		Files: map[string][]byte{"data.txt": []byte("hello")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "env=present")
	require.Contains(t, res.Stdout, "file=hello")
}

func TestNewCanonicalizesCommand(t *testing.T) {
	sb, err := New(WithRuntime("helper", Runtime{
		Command:    " " + os.Args[0] + " ",
		Args:       []string{"-test.run=TestHostHelperProcess", "--"},
		ScriptName: helperScriptName,
	}))
	require.NoError(t, err)
	require.Equal(t, os.Args[0], sb.runtimes["helper"].Command)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "read",
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"TOOLSY_ENV":             "present",
		},
		Files: map[string][]byte{"data.txt": []byte("hello")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "env=present")
	require.Contains(t, res.Stdout, "file=hello")
}

func TestRunSuccessWithEnvAndFiles(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "read",
		Env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"TOOLSY_ENV":             "present",
		},
		Files: map[string][]byte{"data.txt": []byte("hello")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "env=present")
	require.Contains(t, res.Stdout, "file=hello")
}

func TestRunReturnsNonZeroExitAsResult(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "fail",
		Env:      map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	})
	require.NoError(t, err)
	require.Equal(t, 7, res.ExitCode)
	require.Contains(t, res.Stdout, "stdout from helper")
	require.Contains(t, res.Stderr, "stderr from helper")
}

func TestRunRejectsPathTraversal(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "noop",
		Env:      map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
		Files:    map[string][]byte{"../secret.txt": []byte("x")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestRunRejectsReservedScriptNames(t *testing.T) {
	sb, err := New(WithRuntime("helper", Runtime{
		Command:    "true",
		ScriptName: helperScriptName,
	}))
	require.NoError(t, err)

	testCases := []string{
		helperScriptName,
		"dir/../" + helperScriptName,
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := sb.Run(context.Background(), exectool.RunRequest{
				Language: "helper",
				Code:     "noop",
				Files:    map[string][]byte{name: []byte("user data")},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
		})
	}
}

func TestRunReturnsTimeout(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "helper",
		Code:     "sleep",
		Env:      map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunCleansWorkspace(t *testing.T) {
	parent := t.TempDir()
	sb, err := New(
		WithRuntime("helper", helperRuntime()),
		WithTempDirRoot(parent),
	)
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "helper",
		Code:     "noop",
		Env:      map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
	})
	require.NoError(t, err)

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb, err := New(WithRuntime("helper", helperRuntime()))
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestHostHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HOST_CHILD_PROCESS") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}

	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	scriptName := helperScriptName
	if len(os.Args) > 0 {
		last := os.Args[len(os.Args)-1]
		if last != "--" {
			scriptName = last
		}
	}

	code, err := os.ReadFile(scriptName)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(10)
	}

	switch strings.TrimSpace(string(code)) {
	case "read":
		data, err := os.ReadFile(filepath.Join(".", "data.txt"))
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(11)
		}
		_, _ = fmt.Fprintf(os.Stdout, "env=%s file=%s", os.Getenv("TOOLSY_ENV"), string(data))
	case "fail":
		_, _ = fmt.Fprintln(os.Stdout, "stdout from helper")
		_, _ = fmt.Fprintln(os.Stderr, "stderr from helper")
		os.Exit(7)
	case "sleep":
		time.Sleep(5 * time.Second)
	case "spawn-child":
		pidFile := os.Getenv("TOOLSY_CHILD_PID_FILE")
		if pidFile == "" {
			_, _ = fmt.Fprintln(os.Stderr, "missing TOOLSY_CHILD_PID_FILE")
			os.Exit(13)
		}

		child := exec.Command(os.Args[0], "-test.run=TestHostHelperProcess", "--")
		child.Env = append(os.Environ(), "GO_WANT_HOST_CHILD_PROCESS=1")
		if err := child.Start(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(14)
		}

		if err := os.WriteFile(pidFile, fmt.Appendf(nil, "%d", child.Process.Pid), 0o600); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(15)
		}
		for {
			time.Sleep(time.Second)
		}
	case "noop":
		_, _ = fmt.Fprintln(os.Stdout, "ok")
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unknown helper script")
		os.Exit(12)
	}

	os.Exit(0)
}
