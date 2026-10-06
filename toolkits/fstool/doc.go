// Package fstool provides root-relative filesystem tools: list directory,
// read file, and optionally write file. [os.Root] constrains pathname resolution,
// including symlink escape; it does not isolate hardlinks, mounts or host actors.
// The host must control the root and its directory placement. Writes truncate
// and update an existing inode in place, without atomic replacement or durability.
// Read ranges and directory offsets are not snapshots. See README for IO and
// platform limits and host requirements.
package fstool
