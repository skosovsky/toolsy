package toolsygen

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocumentedPresenceFixtureRegeneratesExactly(t *testing.T) {
	// Arrange: the public example is compiled/executed by its own package tests.
	root := findRepoRoot(t)
	example := filepath.Join(root, "examples", "generated_presence")
	manifest, err := os.ReadFile(filepath.Join(example, "presence.json"))
	require.NoError(t, err)
	expected, err := os.ReadFile(filepath.Join(example, "presence_gen.go"))
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "presence.json")
	require.NoError(t, os.WriteFile(path, manifest, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600))
	// Act.
	result, err := Generate(context.Background(), Config{Inputs: []string{path}})
	// Assert: no stale manually maintained generated API representation.
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	actual, err := os.ReadFile(filepath.Join(dir, "presence_gen.go"))
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
}

func TestDocumentedUnsupportedShapesFailBeforeInstallation(t *testing.T) {
	for _, parameters := range []string{
		`{"type":"array","items":{"type":"string"}}`,
		`{"type":["object","null"],"properties":{}}`,
		`{"type":"object","properties":{"value":{"type":"null","description":"unsupported"}}}`,
		`{"type":"object","properties":{"value":{"type":"array","description":"unsupported","items":{"type":["string","null"]}}}}`,
		`{"type":"object","properties":{"value":{"type":"array","description":"unsupported","items":{"type":"array","items":{"type":"string"}}}}}`,
		`{"type":"object","properties":{"value":{"type":"object","description":"unsupported","properties":{}}}}`,
	} {
		t.Run(parameters, func(t *testing.T) {
			// Arrange.
			dir := t.TempDir()
			path := filepath.Join(dir, "unsupported.json")
			require.NoError(
				t,
				os.WriteFile(
					path,
					[]byte(`{"name":"unsupported","description":"documentation boundary","parameters":`+parameters+`}`),
					0o600,
				),
			)
			// Act.
			result, err := Generate(context.Background(), Config{Inputs: []string{path}})
			// Assert.
			require.Error(t, err)
			require.Empty(t, result.Files)
			_, statErr := os.Stat(filepath.Join(dir, "unsupported_gen.go"))
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}
