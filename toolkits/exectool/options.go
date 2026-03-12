package exectool

// Option configures AsTools (enabled languages, output limit, tool names).
type Option func(*options)

type options struct {
	enablePython   bool
	enableBash     bool
	maxOutputBytes int
	pythonName     string
	pythonDesc     string
	bashName       string
	bashDesc       string
}

const defaultMaxOutputBytes = 512 * 1024

func applyDefaults(o *options) {
	if o.maxOutputBytes <= 0 {
		o.maxOutputBytes = defaultMaxOutputBytes
	}
	if o.pythonName == "" {
		o.pythonName = "exec_python"
	}
	if o.pythonDesc == "" {
		o.pythonDesc = "Run a Python script in the sandbox and return stdout/stderr and exit code"
	}
	if o.bashName == "" {
		o.bashName = "exec_bash"
	}
	if o.bashDesc == "" {
		o.bashDesc = "Run a Bash script in the sandbox and return stdout/stderr and exit code"
	}
}

// WithPython enables the exec_python tool.
func WithPython() Option {
	return func(o *options) {
		o.enablePython = true
	}
}

// WithBash enables the exec_bash tool.
func WithBash() Option {
	return func(o *options) {
		o.enableBash = true
	}
}

// WithMaxOutputBytes sets the maximum length for stdout and stderr each (default 512KB).
func WithMaxOutputBytes(n int) Option {
	return func(o *options) {
		o.maxOutputBytes = n
	}
}

// WithPythonName sets the name of the exec_python tool.
func WithPythonName(name string) Option {
	return func(o *options) {
		o.pythonName = name
	}
}

// WithPythonDescription sets the description of the exec_python tool.
func WithPythonDescription(desc string) Option {
	return func(o *options) {
		o.pythonDesc = desc
	}
}

// WithBashName sets the name of the exec_bash tool.
func WithBashName(name string) Option {
	return func(o *options) {
		o.bashName = name
	}
}

// WithBashDescription sets the description of the exec_bash tool.
func WithBashDescription(desc string) Option {
	return func(o *options) {
		o.bashDesc = desc
	}
}
