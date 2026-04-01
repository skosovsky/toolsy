package wazero

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

//go:embed testdata/interpreter.wasm
var interpreterWasm []byte

func TestNewInterpreterRejectsUnsafeLanguage(t *testing.T) {
	_, err := NewInterpreter("wasm", []byte{0x00})
	require.Error(t, err)
}

func TestSupportedLanguagesDoesNotExposeWasm(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)
	require.Equal(t, []string{"jq"}, sb.SupportedLanguages())
}

func TestRunSuccess(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "stdout:hello",
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "hello", res.Stdout)
}

func TestRunMountsFilesAndEnv(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "file:data.txt",
		Files:    map[string][]byte{"data.txt": []byte("payload")},
	})
	require.NoError(t, err)
	require.Equal(t, "payload", res.Stdout)

	res, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "env:NAME",
		Env:      map[string]string{"NAME": "toolsy"},
	})
	require.NoError(t, err)
	require.Equal(t, "toolsy", res.Stdout)
}

func TestRunUsesEngineReportedDuration(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	sb.engine = fakeEngine(
		func(_ context.Context, _ []byte, _ string, _ map[string]string, stdout, _ *bytes.Buffer) (time.Duration, error) {
			time.Sleep(40 * time.Millisecond)
			stdout.WriteString("ok")
			return 25 * time.Millisecond, nil
		},
	)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "noop",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", res.Stdout)
	require.Equal(t, 25*time.Millisecond, res.Duration)
}

func TestRunRejectsReservedScriptNames(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	invoked := false
	sb.engine = fakeEngine(
		func(_ context.Context, _ []byte, _ string, _ map[string]string, _ *bytes.Buffer, _ *bytes.Buffer) (time.Duration, error) {
			invoked = true
			return 0, nil
		},
	)

	testCases := []string{
		"main.code",
		"dir/../main.code",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			invoked = false
			_, err := sb.Run(context.Background(), exectool.RunRequest{
				Language: "jq",
				Code:     "stdout:hello",
				Files:    map[string][]byte{name: []byte("collision")},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
			require.False(t, invoked)
		})
	}
}

func TestRunReturnsNonZeroExitAsResult(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "exit:5",
	})
	require.NoError(t, err)
	require.Equal(t, 5, res.ExitCode)
	require.Contains(t, res.Stderr, "requested exit")
}

func TestRunReturnsTimeout(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "jq",
		Code:     "sleep",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunReturnsTimeoutWhenContextAlreadyExpired(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	require.NotPanics(t, func() {
		_, err = sb.Run(ctx, exectool.RunRequest{
			Language: "jq",
			Code:     "stdout:hello",
		})
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunPropagatesEngineTimeoutPath(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)
	sb.engine = fakeEngine(
		func(ctx context.Context, _ []byte, _ string, _ map[string]string, _ *bytes.Buffer, _ *bytes.Buffer) (time.Duration, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "jq",
		Code:     "noop",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "stdout:x",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestRunWrapsEngineFailures(t *testing.T) {
	sb, err := NewInterpreter("jq", interpreterWasm)
	require.NoError(t, err)
	sb.engine = fakeEngine(
		func(_ context.Context, _ []byte, _ string, _ map[string]string, _ *bytes.Buffer, _ *bytes.Buffer) (time.Duration, error) {
			return 0, errors.New("compile failed")
		},
	)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "jq",
		Code:     "noop",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

type fakeEngine func(ctx context.Context, module []byte, workspaceDir string, env map[string]string, stdout, stderr *bytes.Buffer) (time.Duration, error)

func (f fakeEngine) Run(
	ctx context.Context,
	module []byte,
	workspaceDir string,
	env map[string]string,
	stdout, stderr *bytes.Buffer,
) (time.Duration, error) {
	return f(ctx, module, workspaceDir, env, stdout, stderr)
}
