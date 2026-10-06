package fstool

// Option configures AsTools (read-only mode, limits, tool names and descriptions).
type Option func(*options)

type options struct {
	readOnly       bool
	maxBytes       int
	maxSourceBytes int
	maxEntries     int
	maxScanEntries int
	maxNameBytes   int
	listDirName    string
	listDirDesc    string
	readFileName   string
	readFileDesc   string
	writeFileName  string
	writeFileDesc  string
}

const (
	defaultMaxBytes      = 1024 * 1024 // 1 MB
	defaultListDirName   = "fs_list_dir"
	defaultListDirDesc   = "List files and directories in a path"
	defaultReadFileName  = "fs_read_file"
	defaultReadFileDesc  = "Read contents of a text file"
	defaultWriteFileName = "fs_write_file"
	defaultWriteFileDesc = "Write content to a file (create or overwrite)"
)

func applyDefaults(o *options) {
	if o.maxSourceBytes == 0 {
		o.maxSourceBytes = defaultMaxBytes
	}
	if o.maxEntries == 0 {
		o.maxEntries = 100
	}
	if o.maxScanEntries == 0 {
		o.maxScanEntries = 10000
	}
	if o.maxNameBytes == 0 {
		o.maxNameBytes = 255
	}
	if o.maxBytes == 0 {
		o.maxBytes = defaultMaxBytes
	}
	if o.listDirName == "" {
		o.listDirName = defaultListDirName
	}
	if o.listDirDesc == "" {
		o.listDirDesc = defaultListDirDesc
	}
	if o.readFileName == "" {
		o.readFileName = defaultReadFileName
	}
	if o.readFileDesc == "" {
		o.readFileDesc = defaultReadFileDesc
	}
	if o.writeFileName == "" {
		o.writeFileName = defaultWriteFileName
	}
	if o.writeFileDesc == "" {
		o.writeFileDesc = defaultWriteFileDesc
	}
}

// WithReadOnly sets read-only mode; when true, fs_write_file is not generated.
func WithReadOnly(readOnly bool) Option {
	return func(o *options) {
		o.readOnly = readOnly
	}
}

// WithMaxBytes sets the final JSON byte budget for all successful results (default 1 MiB).
func WithMaxBytes(n int) Option {
	return func(o *options) {
		o.maxBytes = n
	}
}

// WithListDirName sets the name of the list_dir tool.
func WithListDirName(name string) Option {
	return func(o *options) {
		o.listDirName = name
	}
}

// WithListDirDescription sets the description of the list_dir tool.
func WithListDirDescription(desc string) Option {
	return func(o *options) {
		o.listDirDesc = desc
	}
}

// WithReadFileName sets the name of the read_file tool.
func WithReadFileName(name string) Option {
	return func(o *options) {
		o.readFileName = name
	}
}

// WithReadFileDescription sets the description of the read_file tool.
func WithReadFileDescription(desc string) Option {
	return func(o *options) {
		o.readFileDesc = desc
	}
}

// WithWriteFileName sets the name of the write_file tool.
func WithWriteFileName(name string) Option {
	return func(o *options) {
		o.writeFileName = name
	}
}

// WithWriteFileDescription sets the description of the write_file tool.
func WithWriteFileDescription(desc string) Option {
	return func(o *options) {
		o.writeFileDesc = desc
	}
}

// WithMaxSourceBytes bounds read ranges and exact write content (default 1 MiB).
func WithMaxSourceBytes(n int) Option { return func(o *options) { o.maxSourceBytes = n } }

// WithMaxEntries bounds entries returned per page (default 100).
func WithMaxEntries(n int) Option { return func(o *options) { o.maxEntries = n } }

// WithMaxScanEntries bounds directory offset plus lookahead (default 10,000).
func WithMaxScanEntries(n int) Option { return func(o *options) { o.maxScanEntries = n } }

// WithMaxNameBytes bounds each directory entry name (default 255).
func WithMaxNameBytes(n int) Option { return func(o *options) { o.maxNameBytes = n } }
