package e2b

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
	"github.com/skosovsky/toolsy/internal/sandboxfs"
	"github.com/skosovsky/toolsy/textprocessor"
)

type fakeClient struct {
	session     *fakeSession
	err         error
	createCalls int
	createDelay time.Duration
}

func (c *fakeClient) CreateSandbox(context.Context) (Session, error) {
	c.createCalls++
	if c.createDelay > 0 {
		time.Sleep(c.createDelay)
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.session, nil
}

// Test transport fixture streams these bytes; they are not returned in CommandResult.
type fakeCommandOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type fakeSession struct {
	writes       map[string][]byte
	writeErrs    map[string]error
	result       fakeCommandOutput
	err          error
	killed       bool
	killCount    int
	command      string
	args         []string
	mutateArgs   bool
	env          map[string]string
	blockRun     bool
	blockKill    bool
	killErr      error
	writeDelay   time.Duration
	runDelay     time.Duration
	stdoutChunks []string
}

func (s *fakeSession) WriteFile(_ context.Context, path string, data []byte) error {
	if err, ok := s.writeErrs[path]; ok {
		return err
	}
	if s.writes == nil {
		s.writes = make(map[string][]byte)
	}
	if s.writeDelay > 0 {
		time.Sleep(s.writeDelay)
	}
	s.writes[path] = append([]byte(nil), data...)
	return nil
}

func (s *fakeSession) StartAndWait(
	ctx context.Context,
	command string,
	args []string,
	env map[string]string,
	stdout, stderr io.Writer,
) (CommandResult, error) {
	s.command = command
	s.args = append([]string(nil), args...)
	if s.mutateArgs { // Constructor guarantees at least the script argument.
		args[0] = "client changed input"
	}
	if len(env) > 0 {
		s.env = make(map[string]string, len(env))
		maps.Copy(s.env, env)
	}
	if s.blockRun {
		<-ctx.Done()
		return CommandResult{}, ctx.Err()
	}
	if s.runDelay > 0 {
		time.Sleep(s.runDelay)
	}
	if s.err != nil {
		return CommandResult{}, s.err
	}
	if len(s.stdoutChunks) > 0 && stdout != nil {
		for _, chunk := range s.stdoutChunks {
			if _, err := io.WriteString(stdout, chunk); err != nil {
				return CommandResult{}, err
			}
		}
	} else if stdout != nil && s.result.Stdout != "" {
		if _, err := io.WriteString(stdout, s.result.Stdout); err != nil {
			return CommandResult{}, err
		}
	}
	if stderr != nil && s.result.Stderr != "" {
		if _, err := io.WriteString(stderr, s.result.Stderr); err != nil {
			return CommandResult{}, err
		}
	}
	return CommandResult{ExitCode: s.result.ExitCode}, nil
}

func (s *fakeSession) Kill(ctx context.Context) error {
	s.killed = true
	s.killCount++
	if s.blockKill {
		<-ctx.Done()
		return ctx.Err()
	}
	return s.killErr
}

func TestRunSuccess(t *testing.T) {
	session := &fakeSession{result: fakeCommandOutput{Stdout: "ok", ExitCode: 0}}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
		Env:      map[string]string{"MODE": "ci"},
		Files:    map[string][]byte{"data.txt": []byte("payload")},
	})
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Equal(t, "ok", res.Stdout)
	require.Equal(t, "python", session.command)
	require.Equal(t, []string{"/workspace/main.py"}, session.args)
	require.Equal(t, map[string]string{"MODE": "ci"}, session.env)
	require.Equal(t, []byte("payload"), session.writes["/workspace/data.txt"])
	require.Equal(t, []byte("print(1)"), session.writes["/workspace/main.py"])
	require.True(t, session.killed)
}

func TestRunReturnsNonZeroExitAsResult(t *testing.T) {
	session := &fakeSession{result: fakeCommandOutput{Stderr: "boom", ExitCode: 4}}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "bash",
		Code:     "exit 4",
	})
	require.NoError(t, err)
	require.Equal(t, 4, res.ExitCode)
	require.Equal(t, "boom", res.Stderr)
}

func TestRunRejectsUnsupportedLanguage(t *testing.T) {
	sb, err := New(&fakeClient{session: &fakeSession{}})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "ruby",
		Code:     "puts 1",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrUnsupportedLanguage)
}

func TestNewRejectsDuplicateLanguagesAfterTrimming(t *testing.T) {
	_, err := New(
		&fakeClient{session: &fakeSession{}},
		WithRuntime("python", Runtime{Command: "python", Args: []string{"/workspace/main.py"}, ScriptName: "main.py"}),
		WithRuntime(
			" python ",
			Runtime{Command: "python", Args: []string{"/workspace/other.py"}, ScriptName: "other.py"},
		),
	)
	require.Error(t, err)
}

func TestNewRejectsInvalidScriptName(t *testing.T) {
	_, err := New(
		&fakeClient{session: &fakeSession{}},
		WithRuntime(
			"custom",
			Runtime{Command: "python", Args: []string{"/workspace/main.py"}, ScriptName: "../main.py"},
		),
	)
	require.Error(t, err)
}

func TestRunRejectsReservedScriptNames(t *testing.T) {
	client := &fakeClient{session: &fakeSession{}}
	sb, err := New(client)
	require.NoError(t, err)

	testCases := []string{
		"main.py",
		"dir/../main.py",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			client.createCalls = 0
			_, err := sb.Run(context.Background(), exectool.RunRequest{
				Language: "python",
				Code:     "print(1)",
				Files:    map[string][]byte{name: []byte("collision")},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, exectool.ErrSandboxFailure)
			require.Equal(t, 0, client.createCalls)
		})
	}
}

func TestRunReturnsTimeoutAndKillsSession(t *testing.T) {
	session := &fakeSession{blockRun: true}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.True(t, session.killed)
	require.Equal(t, 1, session.killCount)
}

func TestRunWrapsClientFailures(t *testing.T) {
	sb, err := New(&fakeClient{err: errors.New("unavailable")})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
}

func TestRunReturnsTimeoutWhenKillBlocks(t *testing.T) {
	session := &fakeSession{blockRun: true, blockKill: true}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = sb.Run(ctx, exectool.RunRequest{
		Language: "python",
		Code:     "while True: pass",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.True(t, session.killed)
	require.Equal(t, 1, session.killCount)
	// Defer runs cleanupSession with fixed cleanupTimeout; fake Kill blocks until that ctx ends.
	require.Less(t, time.Since(start), cleanupTimeout+200*time.Millisecond)
}

func TestRunReturnsTimeoutWhenFileUploadTimesOut(t *testing.T) {
	session := &fakeSession{
		writeErrs: map[string]error{
			"/workspace/data.txt": context.DeadlineExceeded,
		},
	}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
		Files:    map[string][]byte{"data.txt": []byte("payload")},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.Equal(t, 1, session.killCount)
}

func TestRunReturnsTimeoutWhenScriptUploadTimesOut(t *testing.T) {
	session := &fakeSession{
		writeErrs: map[string]error{
			"/workspace/main.py": context.DeadlineExceeded,
		},
	}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrTimeout)
	require.Equal(t, 1, session.killCount)
}

func TestRunDurationExcludesProvisioningAndUpload(t *testing.T) {
	session := &fakeSession{
		result:     fakeCommandOutput{Stdout: "ok", ExitCode: 0},
		writeDelay: 20 * time.Millisecond,
		runDelay:   30 * time.Millisecond,
	}
	client := &fakeClient{
		session:     session,
		createDelay: 40 * time.Millisecond,
	}
	sb, err := New(client)
	require.NoError(t, err)

	res, err := sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
		Files:    map[string][]byte{"data.txt": []byte("payload")},
	})
	require.NoError(t, err)
	require.Greater(t, res.Duration, 20*time.Millisecond)
	require.Less(t, res.Duration, 70*time.Millisecond)
}

func TestRunRejectsOversizedRemoteOutputStreaming(t *testing.T) {
	chunk := strings.Repeat("x", 64*1024)
	chunks := make([]string, 0, 5)
	for range 5 {
		chunks = append(chunks, chunk)
	}
	session := &fakeSession{stdoutChunks: chunks, result: fakeCommandOutput{ExitCode: 0}}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "byte limit")
}

func TestRunRejectsOversizedRemoteOutput(t *testing.T) {
	big := strings.Repeat("x", sandboxfs.DefaultMaxSandboxOutputBytes+1)
	session := &fakeSession{result: fakeCommandOutput{Stdout: big, ExitCode: 0}}
	sb, err := New(&fakeClient{session: session})
	require.NoError(t, err)

	_, err = sb.Run(context.Background(), exectool.RunRequest{
		Language: "python",
		Code:     "print(1)",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	require.ErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
	require.Contains(t, err.Error(), "byte limit")
}

func TestClassifyControlPlaneError_TimeoutOverReadLimit(t *testing.T) {
	t.Parallel()
	err := classifyControlPlaneError(
		context.Background(),
		fmt.Errorf("start: %w", exectool.ErrTimeout),
		"start process",
	)
	require.ErrorIs(t, err, exectool.ErrTimeout)
}

func TestClassifyControlPlaneError_CancelOverReadLimit(t *testing.T) {
	t.Parallel()
	capErr := fmt.Errorf(
		"%w: stdout exceeds %d byte limit: %w",
		exectool.ErrSandboxFailure,
		4096,
		textprocessor.ErrReadLimitExceeded,
	)
	wrapped := fmt.Errorf("start: %w", context.Canceled)
	inner := fmt.Errorf("setup: %w", errors.Join(wrapped, capErr))
	err := classifyControlPlaneError(context.Background(), inner, "start process")
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunCleanupFailurePreservesResultAndPrimaryError(t *testing.T) {
	for _, primary := range []error{nil, context.Canceled, errors.New("transport failed")} {
		t.Run(fmt.Sprint(primary), func(t *testing.T) {
			// Arrange.
			cleanupErr := errors.New("kill not confirmed")
			session := &fakeSession{
				result:  fakeCommandOutput{Stdout: "done", ExitCode: 7},
				err:     primary,
				killErr: cleanupErr,
			}
			sb, err := New(&fakeClient{session: session})
			require.NoError(t, err)
			// Act.
			result, err := sb.Run(context.Background(), exectool.RunRequest{Language: "python", Code: "print(1)"})
			// Assert.
			require.ErrorIs(t, err, exectool.ErrSandboxCleanup)
			if primary == nil {
				require.Equal(t, 7, result.ExitCode)
				require.Equal(t, "done", result.Stdout)
			} else {
				require.ErrorIs(t, err, primary)
			}
			var diagnostic *exectool.CleanupError
			require.ErrorAs(t, err, &diagnostic)
			require.Equal(t, "e2b", diagnostic.Backend)
			require.ErrorIs(t, diagnostic.Cause, cleanupErr)
		})
	}
}

func TestRuntimeLiteralArgvCanonicalScriptAndOwnership(t *testing.T) {
	// Arrange: shell metacharacters, empty arg and Unicode are literal host config.
	original := []string{"-u", "/workspace/dir/../main script.py", "", "$SECRET", "; echo x", "'quoted'", "界"}
	option := WithRuntime(
		"custom",
		Runtime{Command: "/opt/my python", Args: original, ScriptName: "dir/../main script.py"},
	)
	original[0] = "mutated"
	session := &fakeSession{result: fakeCommandOutput{Stdout: "ok"}, mutateArgs: true}
	sb, err := New(&fakeClient{session: session}, option)
	require.NoError(t, err)
	// Act.
	result, err := sb.Run(t.Context(), exectool.RunRequest{Language: "custom", Code: "print(1)"})
	// Assert: exact script matches the uploaded file, without a shell quoting pass.
	require.NoError(t, err)
	require.Equal(t, "ok", result.Stdout)
	require.Equal(t, "/opt/my python", session.command)
	require.Equal(
		t,
		[]string{"-u", "/workspace/main script.py", "", "$SECRET", "; echo x", "'quoted'", "界"},
		session.args,
	)
	require.Equal(t, []byte("print(1)"), session.writes["/workspace/main script.py"])
	require.Equal(t, "-u", sb.runtimes["custom"].Args[0])
	// A constructed sandbox cannot mutate a reused option snapshot.
	sb.runtimes["custom"].Args[0] = "changed first sandbox"
	second, err := New(&fakeClient{session: &fakeSession{}}, option)
	require.NoError(t, err)
	require.Equal(t, "-u", second.runtimes["custom"].Args[0])
}

func TestRuntimeRejectsMalformedArgv(t *testing.T) {
	for _, runtime := range []Runtime{
		{Command: "", Args: []string{"/workspace/main.py"}, ScriptName: "main.py"},
		{Command: "python", Args: nil, ScriptName: "main.py"},
		{Command: "python", Args: []string{"/workspace/other.py"}, ScriptName: "main.py"},
		{Command: "python", Args: []string{"/workspace/main.py", "/workspace/main.py"}, ScriptName: "main.py"},
		{Command: "python", Args: []string{"/workspace/dir/../main.py", "/workspace/main.py"}, ScriptName: "dir/../main.py"},
		{Command: "py\x00thon", Args: []string{"/workspace/main.py"}, ScriptName: "main.py"},
		{Command: "python", Args: []string{"/workspace/main.py", "bad\x00"}, ScriptName: "main.py"},
		{Command: "python", Args: []string{"/workspace/main.py", string([]byte{0xff})}, ScriptName: "main.py"},
	} {
		// Arrange.
		client := &fakeClient{}
		// Act.
		_, err := New(client, WithRuntime("custom", runtime))
		// Assert: constructor rejection never provisions remote work.
		require.Error(t, err)
		require.Zero(t, client.createCalls)
	}
}
