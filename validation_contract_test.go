package toolsy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateContract_AllPresent(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{
		mustNamedTool(t, "a"),
		mustNamedTool(t, "b"),
	})
	require.NoError(t, ValidateContract(reg, []string{"a", "b"}))
}

func TestValidateContract_EmptyRequired(t *testing.T) {
	reg := mustBuildRegistry(t, nil)
	require.NoError(t, ValidateContract(reg, nil))
	require.NoError(t, ValidateContract(reg, []string{}))
}

func TestValidateContract_MissingTools(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "a")})
	err := ValidateContract(reg, []string{"a", "b", "c"})
	require.Error(t, err)

	var mte *MissingToolsError
	require.ErrorAs(t, err, &mte)
	assert.Equal(t, []string{"a", "b", "c"}, mte.Required)
	assert.Equal(t, []string{"b", "c"}, mte.Missing)
	assert.Contains(t, mte.Error(), "missing required tools")
}

func TestValidateContract_NilRegistry(t *testing.T) {
	err := ValidateContract(nil, []string{"a"})
	require.ErrorIs(t, err, ErrRegistryState)
}

func TestValidateContract_DedupRequiredNames(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "a")})
	err := ValidateContract(reg, []string{"a", "a", "missing", "missing"})
	require.Error(t, err)

	var mte *MissingToolsError
	require.ErrorAs(t, err, &mte)
	assert.Equal(t, []string{"a", "missing"}, mte.Required)
	assert.Equal(t, []string{"missing"}, mte.Missing)
}

func TestValidateContract_InvalidRuntimeState(t *testing.T) {
	var reg Registry
	reg.tools = map[string]Tool{"x": mustNamedTool(t, "x")}

	err := ValidateContract(&reg, []string{"x"})
	require.ErrorIs(t, err, ErrRegistryState)
}
