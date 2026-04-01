package e2b

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
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

type fakeSession struct {
	writes     map[string][]byte
	writeErrs  map[string]error
	result     CommandResult
	err        error
	killed     bool
	killCount  int
	command    string
	env        map[string]string
	blockRun   bool
	blockKill  bool
	writeDelay time.Duration
	runDelay   time.Duration
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

func (s *fakeSession) StartAndWait(ctx context.Context, command string, env map[string]string) (CommandResult, error) {
	s.command = command
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
	return s.result, nil
}

func (s *fakeSession) Kill(ctx context.Context) error {
	s.killed = true
	s.killCount++
	if s.blockKill {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func TestRunSuccess(t *testing.T) {
	session := &fakeSession{result: CommandResult{Stdout: "ok", ExitCode: 0}}
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
	require.Equal(t, "python /workspace/main.py", session.command)
	require.Equal(t, map[string]string{"MODE": "ci"}, session.env)
	require.Equal(t, []byte("payload"), session.writes["/workspace/data.txt"])
	require.Equal(t, []byte("print(1)"), session.writes["/workspace/main.py"])
	require.True(t, session.killed)
}

func TestRunReturnsNonZeroExitAsResult(t *testing.T) {
	session := &fakeSession{result: CommandResult{Stderr: "boom", ExitCode: 4}}
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
		WithRuntime("python", Runtime{Command: "python /workspace/main.py", ScriptName: "main.py"}),
		WithRuntime(" python ", Runtime{Command: "python /workspace/other.py", ScriptName: "other.py"}),
	)
	require.Error(t, err)
}

func TestNewRejectsInvalidScriptName(t *testing.T) {
	_, err := New(
		&fakeClient{session: &fakeSession{}},
		WithRuntime("custom", Runtime{Command: "python /workspace/main.py", ScriptName: "../main.py"}),
	)
	require.Error(t, err)
}

func TestNewNormalizesExactWorkspaceScriptPath(t *testing.T) {
	testCases := []struct {
		name       string
		command    string
		scriptName string
		wantCmd    string
		wantScript string
	}{
		{
			name:       "bare token",
			command:    "python /workspace/dir/../main.py",
			scriptName: "dir/../main.py",
			wantCmd:    "python /workspace/main.py",
			wantScript: "main.py",
		},
		{
			name:       "double quoted",
			command:    `python "/workspace/dir/../main.py"`,
			scriptName: "dir/../main.py",
			wantCmd:    `python "/workspace/main.py"`,
			wantScript: "main.py",
		},
		{
			name:       "single quoted",
			command:    "python '/workspace/dir/../main.py'",
			scriptName: "dir/../main.py",
			wantCmd:    "python '/workspace/main.py'",
			wantScript: "main.py",
		},
		{
			name:       "escaped spaces",
			command:    `python /workspace/dir/../main\ script.py`,
			scriptName: "dir/../main script.py",
			wantCmd:    `python /workspace/main\ script.py`,
			wantScript: "main script.py",
		},
		{
			name:       "rewrite only exact script arg",
			command:    `python "/workspace/dir/../main.py" --cache=/workspace/main.py.cache`,
			scriptName: "dir/../main.py",
			wantCmd:    `python "/workspace/main.py" --cache=/workspace/main.py.cache`,
			wantScript: "main.py",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sb, err := New(
				&fakeClient{session: &fakeSession{}},
				WithRuntime("custom", Runtime{
					Command:    tc.command,
					ScriptName: tc.scriptName,
				}),
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantCmd, sb.runtimes["custom"].Command)
			require.Equal(t, tc.wantScript, sb.runtimes["custom"].ScriptName)
		})
	}
}

func TestNewRejectsAmbiguousScriptPathReferencesInCommand(t *testing.T) {
	testCases := []string{
		"python /workspace/dir/../main.py --cache-key=dir/../main.py",
		"python /workspace/dir/../main.py --output=/tmp/dir/../main.py.bak",
		"python /workspace/dir/../main.py --cache=/workspace/dir/../main.py.cache",
		"python /workspace/dir/../main.py label=/workspace/dir/../main.py",
		"python /workspace/dir/../main.py /workspace/dir/../main.py",
		`sh -c 'python /workspace/dir/../main.py'`,
		`bash -lc "python /workspace/dir/../main.py"`,
		`python "/workspace/dir/../main.py`,
		`python /workspace/dir/../main\`,
	}

	for _, command := range testCases {
		t.Run(command, func(t *testing.T) {
			_, err := New(
				&fakeClient{session: &fakeSession{}},
				WithRuntime("custom", Runtime{
					Command:    command,
					ScriptName: "dir/../main.py",
				}),
			)
			require.Error(t, err)
			require.ErrorContains(t, err, "top-level shell argument")
		})
	}
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
		result:     CommandResult{Stdout: "ok", ExitCode: 0},
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
