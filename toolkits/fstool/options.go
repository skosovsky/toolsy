package fstool

// Option configures AsTools (read-only mode, limits, tool names and descriptions).
type Option func(*options)

type options struct {
	readOnly      bool
	maxBytes      int
	listDirName   string
	listDirDesc   string
	readFileName  string
	readFileDesc  string
	writeFileName string
	writeFileDesc string
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
	if o.maxBytes <= 0 {
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

// WithMaxBytes sets the maximum bytes to read from a file (default 1 MB). Truncation is UTF-8 safe.
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
