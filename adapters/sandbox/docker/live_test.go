//go:build integration

package docker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy/exectool"
)

// TestIntegrationDockerProfile requires a running local Linux Docker daemon and all
// three default images already installed. Unit assertions do not prove isolation.
func TestIntegrationDockerProfile(t *testing.T) {
	// Arrange.
	workspace := t.TempDir()
	sb, err := New(WithWorkspaceRoot(workspace))
	require.NoError(t, err)
	for _, tc := range []struct{ language, code, stdout string }{
		{"python", "print('python-ok')", "python-ok\n"},
		{"bash", "printf 'bash-ok\\n'", "bash-ok\n"},
		{"node", "console.log('node-ok')", "node-ok\n"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Act.
			result, err := sb.Run(ctx, exectool.RunRequest{Language: tc.language, Code: tc.code})
			// Assert.
			require.NoError(t, err)
			require.Equal(t, 0, result.ExitCode)
			require.Equal(t, tc.stdout, result.Stdout)
		})
	}
	t.Run("restrictions", func(t *testing.T) {
		// Arrange.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code := `import os, socket
assert os.getuid() == 65534
status = open('/proc/self/status').read()
assert 'CapEff:\t0000000000000000' in status
assert 'NoNewPrivs:\t1' in status
assert 'Seccomp:\t2' in status
shm = os.statvfs('/dev/shm')
assert shm.f_blocks * shm.f_frsize == 33554432
for path in ['/workspace/main.py','/rootfs-probe']:
    try:
        open(path,'w').write('bad')
    except OSError:
        pass
    else:
        raise AssertionError(path+' writable')
open('/tmp/scratch','w').write('ok')
try:
    socket.create_connection(('1.1.1.1',80),0.2)
except OSError:
    pass
else:
    raise AssertionError('network enabled')
assert open('/sys/fs/cgroup/memory.max').read().strip() == '268435456'
assert open('/sys/fs/cgroup/memory.swap.max').read().strip() == '0'
assert open('/sys/fs/cgroup/pids.max').read().strip() == '64'
assert open('/sys/fs/cgroup/cpu.max').read().strip() == '100000 100000'
try:
    with open('/tmp/fill','wb') as f:
        for i in range(40):
            f.write(b'x' * (1 << 20))
except OSError:
    pass
else:
    raise AssertionError('tmpfs unbounded')
print('restrictions-ok')`
		// Act.
		result, err := sb.Run(ctx, exectool.RunRequest{Language: "python", Code: code})
		// Assert.
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		require.Equal(t, "restrictions-ok\n", result.Stdout)
	})
	t.Run("output-limit", func(t *testing.T) {
		// Arrange.
		p := DefaultPolicy()
		p.OutputBytes = 1024
		limited, err := New(WithPolicy(p), WithWorkspaceRoot(workspace))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Act.
		_, err = limited.Run(ctx, exectool.RunRequest{Language: "python", Code: "print('x' * 2048)"})
		// Assert.
		require.ErrorIs(t, err, exectool.ErrSandboxFailure)
	})
}
