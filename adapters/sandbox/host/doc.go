// Package host provides a local execution sandbox that runs configured host
// binaries inside a temporary workspace.
//
// DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.
//
// On Unix, timeout cleanup kills the entire spawned process group. On other
// platforms, cleanup is best-effort because Go does not expose a portable
// process-tree termination primitive.
package host
