// Package e2b provides a sandbox adapter built around the public E2B concepts
// of creating a sandbox, writing files, running a command, and killing the
// sandbox. Clients must honor contexts and return writer/transport failures.
// Remote infrastructure isolation is a client/service capability, not an adapter guarantee.
package e2b
