package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostReadOnlySQLiteExample(t *testing.T) {
	// Arrange: run owns a disposable SQLite fixture with no external service.
	// Act.
	err := run(t.Context())
	// Assert: SELECT succeeds and the same connection returns SQLITE_READONLY on INSERT.
	require.NoError(t, err)
}
