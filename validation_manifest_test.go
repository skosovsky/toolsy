package toolsy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateManifestContract_AllPresent(t *testing.T) {
	t.Parallel()
	ms, err := NewManifestSet(mustNamedTool(t, "a"), mustNamedTool(t, "b"))
	require.NoError(t, err)
	require.NoError(t, ValidateManifestContract(ms, []string{"a", "b"}))
}

func TestValidateManifestContract_EmptyRequired(t *testing.T) {
	t.Parallel()
	ms, err := ManifestSetFromManifests()
	require.NoError(t, err)
	require.NoError(t, ValidateManifestContract(ms, nil))
	require.NoError(t, ValidateManifestContract(ms, []string{}))
}

func TestValidateManifestContract_MissingTools(t *testing.T) {
	t.Parallel()
	ms, err := NewManifestSet(mustNamedTool(t, "a"))
	require.NoError(t, err)
	err = ValidateManifestContract(ms, []string{"a", "b", "c"})
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeToolsContractMissing, te.Code)
	assert.Equal(t, []string{"b", "c"}, te.FixableArgs)
}

func TestValidateManifestContract_DedupRequiredNames(t *testing.T) {
	t.Parallel()
	ms, err := NewManifestSet(mustNamedTool(t, "a"))
	require.NoError(t, err)
	err = ValidateManifestContract(ms, []string{"a", "a", "missing", "missing"})
	te, ok := AsToolError(err)
	require.True(t, ok)
	require.Equal(t, CodeToolsContractMissing, te.Code)
	assert.Equal(t, []string{"missing"}, te.FixableArgs)
}

func TestValidateManifestContract_FromRegistryManifestSet(t *testing.T) {
	t.Parallel()
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "a"), mustNamedTool(t, "b")})
	ms, err := reg.ManifestSet()
	require.NoError(t, err)
	require.NoError(t, ValidateManifestContract(ms, []string{"a", "b"}))
}

func TestRegistryManifestSet_UnbuiltWithToolsMap(t *testing.T) {
	t.Parallel()
	var reg Registry
	reg.tools = map[string]Tool{"x": mustNamedTool(t, "x")}
	ms, err := reg.ManifestSet()
	require.NoError(t, err)
	require.NoError(t, ValidateManifestContract(ms, []string{"x"}))
}
