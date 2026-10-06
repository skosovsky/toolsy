package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

// This measures an actual checked-in consumer, not a synthetic large registry.
// It does not assert that a deployed host catalog has the same cardinality.
func TestCheckedInCatalogPayload(t *testing.T) {
	// Arrange.
	add, multiply, err := buildTools()
	require.NoError(t, err)
	manifests := []toolsy.ToolManifest{add.Manifest(), multiply.Manifest()}
	// Act.
	raw, err := json.Marshal(manifests) //nolint:musttag // Measures existing Go manifests; does not define a wire DTO.
	require.NoError(t, err)
	var schemaBytes int
	for _, manifest := range manifests {
		parameters, encodeErr := json.Marshal(manifest.Parameters)
		require.NoError(t, encodeErr)
		output, encodeErr := json.Marshal(manifest.OutputSchema)
		require.NoError(t, encodeErr)
		schemaBytes += len(parameters) + len(output)
	}
	// Assert: report measured sizes; large-catalog activation is a host decision.
	require.NotEmpty(t, raw)
	t.Logf("tools=%d full_catalog_bytes=%d schema_bytes=%d", len(manifests), len(raw), schemaBytes)
}

func BenchmarkCheckedInCatalogSchemaPayload(b *testing.B) {
	add, multiply, err := buildTools()
	require.NoError(b, err)
	b.ReportAllocs()
	for b.Loop() {
		// Include loading defensive manifest/schema snapshots and encoding them.
		//nolint:musttag // Measures existing Go manifests; does not define a wire DTO.
		_, encodeErr := json.Marshal([]toolsy.ToolManifest{add.Manifest(), multiply.Manifest()})
		require.NoError(b, encodeErr)
	}
}
