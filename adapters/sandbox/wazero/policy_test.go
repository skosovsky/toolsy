package wazero

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	runtimewazero "github.com/tetratelabs/wazero"
	wazerosys "github.com/tetratelabs/wazero/sys"

	"github.com/skosovsky/toolsy/exectool"
)

// (module (memory (export "memory") 1)
//
//	(func (export "grow") (result i32) i32.const 1 memory.grow))
func memoryGrowthModule() []byte {
	return []byte{
		0, 97, 115, 109, 1, 0, 0, 0,
		1, 5, 1, 96, 0, 1, 127,
		3, 2, 1, 0,
		5, 3, 1, 0, 1,
		7, 17, 2, 6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0, 4, 'g', 'r', 'o', 'w', 0, 0,
		10, 8, 1, 6, 0, 65, 1, 64, 0, 11,
	}
}

func TestMemoryBudgetEnforcedWithCustomConfig(t *testing.T) {
	// Arrange.
	sb, err := NewInterpreter("jq", memoryGrowthModule(),
		WithRuntimeConfig(runtimewazero.NewRuntimeConfigInterpreter().WithMemoryLimitPages(100)),
		WithMemoryLimitPages(1))
	require.NoError(t, err)
	ctx := context.Background()
	runtime := runtimewazero.NewRuntimeWithConfig(ctx, sb.engine.(*wazeroEngine).runtimeConfig)
	defer func() { require.NoError(t, runtime.Close(ctx)) }()
	mod, err := runtime.Instantiate(ctx, memoryGrowthModule())
	require.NoError(t, err)
	// Act: request a second page in the real runtime.
	result, err := mod.ExportedFunction("grow").Call(ctx)
	// Assert: growth rejected, one page retained.
	require.NoError(t, err)
	require.Equal(t, []uint64{0xffffffff}, result)
	require.Equal(t, uint32(65536), mod.Memory().Size())
}

func TestMemoryBudgetRejectsInvalidLimits(t *testing.T) {
	for _, pages := range []uint32{0, 65537} {
		t.Run(strconv.FormatUint(uint64(pages), 10), func(t *testing.T) {
			// Arrange and Act.
			_, err := NewInterpreter("jq", interpreterWasm, WithMemoryLimitPages(pages))
			// Assert.
			require.ErrorContains(t, err, "memory limit")
		})
	}
}

func TestCustomConfigCancellationSubprocess(t *testing.T) {
	if os.Getenv("TOOLSY_WAZERO_CANCELLATION_CHILD") == "1" {
		// Arrange: adapter must override the host's disabled cancellation.
		sb, err := NewInterpreter(
			"jq",
			infiniteLoopModule(),
			WithRuntimeConfig(
				runtimewazero.NewRuntimeConfigInterpreter().WithCloseOnContextDone(false).WithMemoryLimitPages(2048),
			),
			WithMemoryLimitPages(1024),
		)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		// Act: the _start function loops without a host call.
		_, err = sb.Run(ctx, exectool.RunRequest{Language: "jq", Code: "sleep"})
		// Assert.
		require.ErrorIs(t, err, exectool.ErrTimeout)
		return
	}
	// Arrange: process deadline keeps a regression from hanging the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestCustomConfigCancellationSubprocess$",
		"-test.timeout=8s",
	)
	cmd.Env = append(os.Environ(), "TOOLSY_WAZERO_CANCELLATION_CHILD=1")
	// Act.
	output, err := cmd.CombinedOutput()
	// Assert.
	require.NoError(t, ctx.Err(), string(output))
	require.NoError(t, err, string(output))
}

func TestCleanupDiagnosticPreservesGuestOutcome(t *testing.T) {
	for _, exitCode := range []uint32{0, 5} {
		t.Run(strconv.FormatUint(uint64(exitCode), 10), func(t *testing.T) {
			// Arrange: a completed guest plus a failed runtime close.
			sb, err := NewInterpreter("jq", interpreterWasm)
			require.NoError(t, err)
			cause := errors.New("close failed")
			sb.engine = fakeEngine(
				func(_ context.Context, _ []byte, _ string, _ map[string]string, stdout, _ io.Writer) (time.Duration, error) {
					_, _ = io.WriteString(stdout, "complete")
					var primary error
					if exitCode != 0 {
						primary = wazerosys.NewExitError(exitCode)
					}
					return time.Millisecond, &engineCleanupError{
						primary: primary,
						cleanup: &exectool.CleanupError{Backend: "wazero", Operation: "close runtime", Cause: cause},
					}
				},
			)
			// Act.
			result, err := sb.Run(context.Background(), exectool.RunRequest{Language: "jq"})
			// Assert: complete output and exit code survive, cleanup stays inspectable.
			require.Equal(t, "complete", result.Stdout)
			require.Equal(t, int(exitCode), result.ExitCode)
			diagnostic, ok := errors.AsType[*exectool.CleanupError](err)
			require.True(t, ok)
			require.Equal(t, cause, diagnostic.Cause)
			require.ErrorIs(t, err, exectool.ErrSandboxCleanup)
		})
	}
}

// (module (memory 1) (func (export "_start") (loop br 0))).
func infiniteLoopModule() []byte {
	return []byte{
		0, 97, 115, 109, 1, 0, 0, 0,
		1, 4, 1, 96, 0, 0,
		3, 2, 1, 0,
		5, 3, 1, 0, 1,
		7, 10, 1, 6, '_', 's', 't', 'a', 'r', 't', 0, 0,
		10, 9, 1, 7, 0, 3, 64, 12, 0, 11, 11,
	}
}

func TestGuestWorkspaceContainment(t *testing.T) {
	// Arrange: outside secret, in-workspace positive control, and escaping symlink.
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0700))
	secret := filepath.Join(parent, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside-secret"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "allowed.txt"), []byte("inside"), 0600))
	require.NoError(t, os.Symlink(secret, filepath.Join(workspace, "escape.txt")))
	sb, err := NewInterpreter("jq", interpreterWasm, WithRuntimeConfig(runtimewazero.NewRuntimeConfigInterpreter()))
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		want     string
		wantExit uint32
	}{
		{name: "allowed.txt", want: "inside", wantExit: 0},
		{name: "../secret.txt", want: "", wantExit: 3},
		{name: "escape.txt", want: "", wantExit: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(workspace, "main.code"), []byte("file:"+tc.name), 0600))
			var stdout, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Act: actual WASI guest opens the requested path.
			_, err := sb.engine.Run(ctx, sb.module, workspace, nil, &stdout, &stderr)
			// Assert: only the confined positive control can read content.
			require.Equal(t, tc.want, stdout.String())
			if tc.wantExit == 0 {
				require.NoError(t, err)
			} else {
				guestExit, ok := errors.AsType[*wazerosys.ExitError](err)
				require.True(t, ok, "%v", err)
				require.Equal(t, tc.wantExit, guestExit.ExitCode())
			}
		})
	}
}
