// Package host provides a local execution sandbox that runs configured host
// binaries inside a temporary workspace.
//
// DANGER: NO ISOLATION. USE ONLY WITH HUMAN-IN-THE-LOOP.
//
// Guest environment inheritance is disabled unless explicitly configured.
// This backend is for trusted code: it provides no filesystem, resource or network isolation.
//
// On Unix, timeout cleanup kills the launched process group. On other
// platforms, cleanup is best-effort because Go does not expose a portable
// process-tree termination primitive.
package host
