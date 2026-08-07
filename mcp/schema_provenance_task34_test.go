package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	task34SchemaCommit = "271ecc9accafdd9b83a3c869fa67c22953b2af80"
	task34SchemaSHA256 = "ef70b61f99b6d2e5e3b46863822eab08dff6a45bedc7a08914e0e5b133f40203"
)

type task34SchemaProvenance struct {
	ProtocolRevision string `json:"protocolRevision"`
	Artifact         string `json:"artifact"`
	ArtifactBytes    int    `json:"artifactBytes"`
	SHA256           string `json:"sha256"`
	SourceCommit     string `json:"sourceCommit"`
	SourcePath       string `json:"sourcePath"`
	SourceURL        string `json:"sourceURL"`
	FixtureContract  struct {
		Official    []string `json:"official"`
		Adversarial []string `json:"adversarial"`
	} `json:"fixtureContract"`
}

func TestTask34OfficialSchemaArtifactHasImmutableProvenance(t *testing.T) {
	// Arrange.
	schemaDirectory := filepath.Join("testdata", "task34", "schema")
	manifestRaw, err := os.ReadFile(filepath.Join(schemaDirectory, "PROVENANCE.json"))
	require.NoError(t, err)
	var manifest task34SchemaProvenance
	require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
	artifactRaw, err := os.ReadFile(filepath.Join(schemaDirectory, manifest.Artifact))
	require.NoError(t, err)

	// Act.
	digest := sha256.Sum256(artifactRaw)
	actualSHA256 := hex.EncodeToString(digest[:])
	var schema struct {
		Dialect string                     `json:"$schema"`
		Defs    map[string]json.RawMessage `json:"$defs"`
	}
	schemaErr := json.Unmarshal(artifactRaw, &schema)
	fixturePaths := append(
		append([]string(nil), manifest.FixtureContract.Official...),
		manifest.FixtureContract.Adversarial...,
	)

	// Assert.
	require.Equal(t, ProtocolVersion, manifest.ProtocolRevision)
	require.Equal(t, task34SchemaCommit, manifest.SourceCommit)
	require.Equal(t, "schema/2026-07-28/schema.json", manifest.SourcePath)
	require.Contains(t, manifest.SourceURL, task34SchemaCommit)
	require.Equal(t, task34SchemaSHA256, manifest.SHA256)
	require.Equal(t, task34SchemaSHA256, actualSHA256)
	require.Len(t, artifactRaw, manifest.ArtifactBytes)
	require.NoError(t, schemaErr)
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema.Dialect)
	require.Contains(t, schema.Defs, "DiscoverRequest")
	require.Contains(t, schema.Defs, "DiscoverResult")
	require.Contains(t, schema.Defs, "SubscriptionsListenResultResponse")
	require.Len(t, fixturePaths, 19)
	seen := make(map[string]struct{}, len(fixturePaths))
	for _, relative := range fixturePaths {
		_, duplicate := seen[relative]
		require.False(t, duplicate, relative)
		seen[relative] = struct{}{}
		info, statErr := os.Stat(filepath.Clean(filepath.Join(schemaDirectory, relative)))
		require.NoError(t, statErr, relative)
		require.False(t, info.IsDir(), relative)
	}
}
