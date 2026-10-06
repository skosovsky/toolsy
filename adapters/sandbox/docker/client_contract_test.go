package docker_test

import (
	sdk "github.com/docker/docker/client"

	"github.com/skosovsky/toolsy/adapters/sandbox/docker"
)

// Compile the exported port from an external package without a library shim.
var _ docker.Client = (*sdk.Client)(nil)
