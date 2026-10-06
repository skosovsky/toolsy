package host

import "maps"

// Runtime describes how a language should be executed on the host.
type Runtime struct {
	Command    string
	Args       []string
	ScriptName string
}

// Option configures the host sandbox.
type Option func(*options)

type options struct {
	runtimes           map[string]Runtime
	tempDirRoot        string
	environment        map[string]string
	inheritEnvironment bool
}

// WithRuntime adds or overrides a language runtime mapping.
func WithRuntime(language string, runtime Runtime) Option {
	return func(o *options) {
		if o.runtimes == nil {
			o.runtimes = make(map[string]Runtime)
		}
		o.runtimes[language] = runtime
	}
}

// WithTempDirRoot overrides the parent directory used for temporary
// workspaces. It is primarily useful for tests and controlled environments.
func WithTempDirRoot(root string) Option {
	return func(o *options) {
		o.tempDirRoot = root
	}
}

// WithEnvironment sets the host-selected base environment. Request variables override it.
// The map is copied when the option is created.
func WithEnvironment(env map[string]string) Option {
	snapshot := maps.Clone(env)
	return func(o *options) { o.environment = snapshot }
}

// WithInheritedEnvironment explicitly enables inheriting the parent environment.
// This may expose credentials to guest code. WithEnvironment and request Env override it.
func WithInheritedEnvironment() Option {
	return func(o *options) { o.inheritEnvironment = true }
}
