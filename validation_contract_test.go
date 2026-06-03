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
	requireToolsContractMissing(t, err, []string{"a", "b", "c"}, []string{"b", "c"})
}

func TestValidateContract_NilRegistry(t *testing.T) {
	err := ValidateContract(nil, []string{"a"})
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)
}

func TestValidateContract_DedupRequiredNames(t *testing.T) {
	reg := mustBuildRegistry(t, []Tool{mustNamedTool(t, "a")})
	err := ValidateContract(reg, []string{"a", "a", "missing", "missing"})
	requireToolsContractMissing(t, err, []string{"a", "missing"}, []string{"missing"})
}

func TestValidateContract_InvalidRuntimeState(t *testing.T) {
	var reg Registry
	reg.tools = map[string]Tool{"x": mustNamedTool(t, "x")}

	err := ValidateContract(&reg, []string{"x"})
	requireToolErrorCode(t, err, CodeRegistryNotReady, ErrRegistryState)
}

func requireToolsContractMissing(t *testing.T, err error, required, missing []string) {
	t.Helper()
	te, ok := AsToolError(err)
	require.True(t, ok, "expected ToolError, got %T: %v", err, err)
	require.Equal(t, CodeToolsContractMissing, te.Code)
	assert.Equal(t, missing, te.FixableArgs)
	assert.Contains(t, te.Reason, "missing required tools")
	for _, name := range required {
		assert.Contains(t, te.Reason, name)
	}
}
