package e2b

// Runtime describes how a language should be executed inside the remote E2B
// workspace.
//
// Command must be a simple POSIX-style command line where the script path
// appears exactly once as a top-level shell argument. Wrapper forms such as
// `sh -c 'python /workspace/main.py'` are intentionally unsupported.
type Runtime struct {
	Command    string
	ScriptName string
}

// Option configures the E2B sandbox adapter.
type Option func(*options)

type options struct {
	runtimes map[string]Runtime
}

// WithRuntime adds or overrides a language runtime mapping.
//
// The supplied Runtime.Command is validated by New against the supported
// top-level script-argument subset described on Runtime.
func WithRuntime(language string, runtime Runtime) Option {
	return func(o *options) {
		if o.runtimes == nil {
			o.runtimes = defaultRuntimes()
		}
		o.runtimes[language] = runtime
	}
}

func defaultRuntimes() map[string]Runtime {
	return map[string]Runtime{
		"bash": {
			Command:    "bash /workspace/main.sh",
			ScriptName: "main.sh",
		},
		"go": {
			Command:    "go run /workspace/main.go",
			ScriptName: "main.go",
		},
		"js": {
			Command:    "node /workspace/main.js",
			ScriptName: "main.js",
		},
		"python": {
			Command:    "python /workspace/main.py",
			ScriptName: "main.py",
		},
	}
}
