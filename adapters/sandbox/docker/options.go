package docker

import (
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
)

const (
	languageBash   = "bash"
	languagePython = "python"
)

// Runtime describes a language-specific Docker execution template.
type Runtime struct {
	Image      string
	Command    []string
	ScriptName string
}

// Policy contains mandatory limits. Zero/negative limits are rejected.
// Workspace is immutable; TmpBytes limits /tmp and, independently, /dev/shm.
type Policy struct {
	MemoryBytes int64
	CPUQuota    int64
	PIDs        int64
	TmpBytes    int64
	OutputBytes int
	InputBytes  int
	MaxFiles    int
	LogTimeout  time.Duration
}

// DefaultPolicy disables networking and bounds memory, CPU, processes and writable scratch.
func DefaultPolicy() Policy {
	const (
		defaultMemory   = 256 << 20
		defaultCPUQuota = 100000
		defaultPIDs     = 64
		defaultTmp      = 32 << 20
		defaultOutput   = 8 << 20
		defaultInput    = 64 << 20
		defaultFiles    = 256
	)
	return Policy{
		MemoryBytes: defaultMemory,
		CPUQuota:    defaultCPUQuota,
		PIDs:        defaultPIDs,
		TmpBytes:    defaultTmp,
		OutputBytes: defaultOutput,
		InputBytes:  defaultInput,
		MaxFiles:    defaultFiles,
		LogTimeout:  logsTimeout,
	}
}

// Option configures the Docker sandbox.
type Option func(*options)
type options struct {
	runtimes      map[string]Runtime
	policy        Policy
	workspaceRoot string
	client        dockerClient
}

// WithPolicy selects mandatory enforceable bounds; CPUQuota is microseconds per 100ms period.
func WithPolicy(policy Policy) Option { return func(o *options) { o.policy = policy } }

// WithWorkspaceRoot selects a local directory visible at the SAME path to the Docker daemon.
// A remote daemon cannot use this adapter's immutable local workspace contract.
func WithWorkspaceRoot(root string) Option { return func(o *options) { o.workspaceRoot = root } }

// WithImageMapping overrides runtime images. Images must support the unprivileged readonly profile.
func WithImageMapping(images map[string]string) Option {
	return func(o *options) {
		for language, image := range images {
			r := o.runtimes[language]
			r.Image = image
			o.runtimes[language] = r
		}
	}
}

// WithMemoryLimit selects a mandatory memory bound; nonpositive values are rejected.
func WithMemoryLimit(bytes int64) Option { return func(o *options) { o.policy.MemoryBytes = bytes } }

// WithClient injects a client. Info must truthfully report mandatory cgroup capabilities;
// the client must target a local daemon with access to the workspace paths.
func WithClient(client dockerClient) Option { return func(o *options) { o.client = client } }
func defaultRuntimes() map[string]Runtime {
	return map[string]Runtime{
		languageBash: {
			Image:      "bash:5.2",
			Command:    []string{languageBash, "/workspace/main.sh"},
			ScriptName: "main.sh",
		},
		"node": {
			Image:      "node:22-alpine",
			Command:    []string{"node", "/workspace/main.js"},
			ScriptName: "main.js",
		},
		languagePython: {
			Image:      "python:3.11-alpine",
			Command:    []string{languagePython, "/workspace/main.py"},
			ScriptName: "main.py",
		},
	}
}
func hostConfig(p Policy, workspace string) *container.HostConfig {
	const cpuPeriod = 100000
	var hc container.HostConfig
	hc.NetworkMode = "none"
	hc.ReadonlyRootfs = true
	hc.ShmSize = p.TmpBytes
	hc.CapDrop = []string{"ALL"}
	hc.SecurityOpt = []string{"no-new-privileges:true"}
	hc.Binds = []string{workspace + ":/workspace:ro"}
	hc.Tmpfs = map[string]string{
		"/tmp": "rw,noexec,nosuid,nodev,size=" + strconv.FormatInt(p.TmpBytes, 10) + ",mode=1777",
	}
	hc.LogConfig.Type = "json-file"
	hc.Memory = p.MemoryBytes
	hc.MemorySwap = p.MemoryBytes
	hc.CPUPeriod = cpuPeriod
	hc.CPUQuota = p.CPUQuota
	hc.PidsLimit = &p.PIDs
	return &hc
}
