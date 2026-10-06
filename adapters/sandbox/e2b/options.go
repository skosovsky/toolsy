package e2b

const (
	languagePython   = "python"
	pythonCommand    = "python"
	pythonScriptName = "main.py"
)

// Runtime describes how a language should be executed inside the remote E2B
// workspace.
//
// Command is a literal executable; Args must contain exactly one literal
// /workspace/<ScriptName> argument. New canonicalizes that argument only.
// Other arguments are trusted host configuration, never parsed as shell syntax.
type Runtime struct {
	Command    string
	Args       []string
	ScriptName string
}

// Option configures the E2B sandbox adapter.
type Option func(*options)

type options struct {
	runtimes map[string]Runtime
}

// WithRuntime adds or overrides a language runtime mapping.
//
// Args are captured at option creation and copied again by New and each dispatch.
func WithRuntime(language string, runtime Runtime) Option {
	runtime.Args = append([]string(nil), runtime.Args...)
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
			Command:    "bash",
			Args:       []string{"/workspace/main.sh"},
			ScriptName: "main.sh",
		},
		"go": {
			Command:    "go",
			Args:       []string{"run", "/workspace/main.go"},
			ScriptName: "main.go",
		},
		"js": {
			Command:    "node",
			Args:       []string{"/workspace/main.js"},
			ScriptName: "main.js",
		},
		languagePython: {
			Command:    pythonCommand,
			Args:       []string{workspacePrefix + pythonScriptName},
			ScriptName: pythonScriptName,
		},
	}
}
