package sqltool

// Option configures AsTools (row/cell limits, allowed tables, tool names).
type Option func(*options)

type options struct {
	maxRows             int
	maxCellBytes        int
	maxSchemaBytes      int
	allowedTables       []string
	inspectName         string
	inspectDesc         string
	executeName         string
	executeDesc         string
	inspectFormatter    func(InspectResult) (any, error)
	executeFormatter    func(ExecuteResult) (any, error)
	hostResultValidator func(any) error
}

const (
	defaultMaxRows        = 100
	defaultMaxCellBytes   = 200
	defaultMaxSchemaBytes = 512 * 1024 // 512 KB
	defaultInspectName    = "sql_inspect_schema"
	defaultInspectDesc    = "Get DDL/schema of allowed tables"
	defaultExecuteName    = "sql_execute_read"
	defaultExecuteDesc    = "Execute a SELECT query and return results as a table"
)

func applyDefaults(o *options) {
	if o.maxRows <= 0 {
		o.maxRows = defaultMaxRows
	}
	if o.maxCellBytes <= 0 {
		o.maxCellBytes = defaultMaxCellBytes
	}
	if o.maxSchemaBytes <= 0 {
		o.maxSchemaBytes = defaultMaxSchemaBytes
	}
	if o.inspectName == "" {
		o.inspectName = defaultInspectName
	}
	if o.inspectDesc == "" {
		o.inspectDesc = defaultInspectDesc
	}
	if o.executeName == "" {
		o.executeName = defaultExecuteName
	}
	if o.executeDesc == "" {
		o.executeDesc = defaultExecuteDesc
	}
}

// WithMaxRows sets the maximum number of rows returned by sql_execute_read (default 100).
func WithMaxRows(n int) Option {
	return func(o *options) {
		o.maxRows = n
	}
}

// WithMaxCellBytes sets the maximum length per cell value to avoid context blowup (default 200).
func WithMaxCellBytes(n int) Option {
	return func(o *options) {
		o.maxCellBytes = n
	}
}

// WithMaxSchemaBytes sets the final wire JSON byte budget for sql_inspect_schema (default 512 KB).
// Schema builder output is not suffix-truncated; wire suffix applies via format.CapWireJSON only.
func WithMaxSchemaBytes(n int) Option {
	return func(o *options) {
		o.maxSchemaBytes = n
	}
}

// WithAllowedTables restricts schema inspection to these table names. Empty means no filter.
func WithAllowedTables(tables []string) Option {
	return func(o *options) {
		o.allowedTables = tables
	}
}

// WithInspectName sets the name of the inspect_schema tool.
func WithInspectName(name string) Option {
	return func(o *options) {
		o.inspectName = name
	}
}

// WithInspectDescription sets the description of the inspect_schema tool.
func WithInspectDescription(desc string) Option {
	return func(o *options) {
		o.inspectDesc = desc
	}
}

// WithExecuteName sets the name of the execute_read tool.
func WithExecuteName(name string) Option {
	return func(o *options) {
		o.executeName = name
	}
}

// WithExecuteDescription sets the description of the execute_read tool.
func WithExecuteDescription(desc string) Option {
	return func(o *options) {
		o.executeDesc = desc
	}
}

// WithInspectResultFormatter overrides JSON output for sql_inspect_schema.
func WithInspectResultFormatter(f func(InspectResult) (any, error)) Option {
	return func(o *options) {
		o.inspectFormatter = f
	}
}

// WithExecuteResultFormatter overrides JSON output for sql_execute_read.
func WithExecuteResultFormatter(f func(ExecuteResult) (any, error)) Option {
	return func(o *options) {
		o.executeFormatter = f
	}
}

// WithHostResultValidator validates formatted tool output before JSON marshal.
func WithHostResultValidator(v func(any) error) Option {
	return func(o *options) {
		o.hostResultValidator = v
	}
}
