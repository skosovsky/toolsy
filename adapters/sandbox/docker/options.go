package docker

import "github.com/docker/docker/api/types/container"

// Runtime describes a language-specific Docker execution template.
type Runtime struct {
	Image      string
	Command    []string
	ScriptName string
}

// Option configures the Docker sandbox.
type Option func(*options)

type options struct {
	runtimes        map[string]Runtime
	networkDisabled bool
	memoryLimit     int64
	client          dockerClient
}

// WithImageMapping overrides runtime images for supported languages.
func WithImageMapping(images map[string]string) Option {
	return func(o *options) {
		if o.runtimes == nil {
			o.runtimes = defaultRuntimes()
		}
		for language, image := range images {
			runtime := o.runtimes[language]
			runtime.Image = image
			o.runtimes[language] = runtime
		}
	}
}

// WithNetworkDisabled disables network access for created containers.
func WithNetworkDisabled() Option {
	return func(o *options) {
		o.networkDisabled = true
	}
}

// WithMemoryLimit caps container memory in bytes.
func WithMemoryLimit(bytes int64) Option {
	return func(o *options) {
		o.memoryLimit = bytes
	}
}

// WithClient injects a docker client implementation, primarily for tests.
func WithClient(client dockerClient) Option {
	return func(o *options) {
		o.client = client
	}
}

func defaultRuntimes() map[string]Runtime {
	return map[string]Runtime{
		"bash": {
			Image:      "bash:5.2",
			Command:    []string{"bash", "/workspace/main.sh"},
			ScriptName: "main.sh",
		},
		"node": {
			Image:      "node:22-alpine",
			Command:    []string{"node", "/workspace/main.js"},
			ScriptName: "main.js",
		},
		"python": {
			Image:      "python:3.11-alpine",
			Command:    []string{"python", "/workspace/main.py"},
			ScriptName: "main.py",
		},
	}
}

func hostConfig(networkDisabled bool, memoryLimit int64) *container.HostConfig {
	var hc container.HostConfig
	if networkDisabled {
		hc.NetworkMode = "none"
	}
	if memoryLimit > 0 {
		hc.Memory = memoryLimit
	}
	return &hc
}
