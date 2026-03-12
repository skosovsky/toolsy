// Package fstool provides a sandboxed filesystem toolkit for agents: list directory,
// read file, and optionally write file. All paths are validated against a base
// directory to prevent path traversal and symlink escape.
package fstool
