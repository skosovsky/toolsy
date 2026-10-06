# Filesystem toolkit

`AsTools(baseDir, opts...)` registers list/read and optionally write tools. The host selects an existing base directory; model paths are relative, and absolute paths and `..` segments are rejected. Each execution opens `os.Root` and uses its root-relative open/mkdir operations, including symlink containment at actual access. Outside symlinks cannot be followed, including when replaced concurrently. The host controls the base directory itself: do not allow another actor to replace the host root or move its directories outside it. This is not isolation from hard links, mount points, special files, or malicious host filesystem administration. Only regular files are read/written. Unix opens are nonblocking to reject FIFOs without waiting for a peer; on other platforms the host must exclude device paths (their open semantics are not bounded). Successful writes create parent directories and write the exact approved content without shortening it. Writes truncate and update the existing file in place; neither single-file replacement nor multi-file updates are atomic. Confirmation metadata is not an authorization grant.

## Limits and continuation

All limits are finite; zero selects the default, negative options reject construction. The source cap must leave room for one overflow-probe byte, and the entry cap must be smaller than the scan cap. `WithMaxBytes` (1 MiB) bounds every successful final JSON result **after escaping**. Oversized results fail validation and emit no successful result. `WithMaxSourceBytes` (1 MiB) bounds a read range and write content. `WithMaxEntries` (100) bounds returned directory entries, `WithMaxScanEntries` (10,000) bounds offset plus lookahead, and `WithMaxNameBytes` (255) bounds each entry name. Names are returned without content shortening. Directory reads allocate only a bounded batch; no full-directory sorting/collection.

`fs_read_file` takes `path`, optional byte `offset`, and optional byte `length` (zero uses the source limit). It returns `path`, `content`, `next_offset`, and `has_more`. File ranges are not snapshots; next offsets use returned byte counts and has_more reflects the size observed at open, so concurrent modification can change subsequent contents. Ranges must end on UTF-8 boundaries; invalid UTF-8 fails validation. Without an explicit length, files beyond the source limit fail validation; use an explicit bounded range to continue. `fs_list_dir` takes `path`, optional `offset` and `limit` (zero uses the entry limit), and returns `path`, `entries`, `next_offset`, and `has_more`. Offset is the actual filesystem enumeration position, not a sorted or snapshot cursor; changes between calls may reorder, duplicate, or omit entries. When the scan ceiling is reached, continuation fails explicitly. Select a smaller range/page if wire JSON does not fit. `fs_write_file` takes `path` and `content` and returns `status`.

Read-only configuration (`WithReadOnly(true)`) omits the write tool. Tool names/descriptions can be overridden with the corresponding options. Root handles are closed after each execution; tools need no separate lifecycle API. Cancellation is checked between local IO operations; local file IO does not provide asynchronous interruption. Symlinks within the root are allowed; absolute symlinks are rejected by os.Root even if their target is inside the root.


Nil options reject construction. Host ports and callbacks are borrowed; the host
owns their lifetime and synchronization. See the [shared constructor and ownership
contract](../README.md#constructor-configuration-and-ownership) for option snapshots
and the distinction between configuration containers and mutable host ports.


## Filesystem boundary and host consistency

`os.Root` constrains path resolution; it is not a filesystem isolation boundary.
A regular file hardlinked inside the root may refer to the same inode as a name
outside it: reading exposes those bytes and writing changes both names. Mounts
inside the root expose their mounted contents. The toolkit does not enumerate or
reject hardlinks or mount points. The host must provision a tree without unwanted
aliases/mounts and prevent untrusted changes to the root and directory placement.
`WithReadOnly(true)` omits the write tool; it does not restrict other host actors.

`fs_write_file` opens a regular file, truncates it to zero, then writes content.
Concurrent readers can see an empty or partial file, and a failure after truncation
can leave changed bytes or created parent directories. There is no rollback or
compare-and-swap version check. The success status confirms the write calls, not
crash durability: there is no file/directory sync and deferred close errors are
not reported. A later delivery error also does not undo the write. Host recovery
must inspect the actual state before authorizing a retry.

If stable read ranges or directory pagination are required, use an immutable
versioned tree or a host-owned snapshot kept stable for the whole sequence. A
numeric offset is not a snapshot token. If atomic or crash-durable replacement is
required, provide a separate host-authorized operation using a temporary regular
file on the same filesystem, a checked write/close and atomic rename, plus the
filesystem-specific file/directory sync protocol. Keep ownership and authorization
checks at that host boundary; this toolkit does not implement a transaction or
storage lifecycle manager.

The public limitation fixtures in `isolation_contract_test.go` exercise hardlink
aliasing, existing inode updates and mutation between read ranges on a disposable
local tree. They demonstrate the documented boundary; they do not certify mount
isolation, filesystem crash recovery or behavior on every operating system.
