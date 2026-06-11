package toolsy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManifestSet_SingleTool(t *testing.T) {
	t.Parallel()
	toolA := mustNamedTool(t, "a")
	ms, err := NewManifestSet(toolA)
	require.NoError(t, err)
	assert.True(t, ms.Has("a"))
	assert.Equal(t, []string{"a"}, ms.Names())
}

func TestNewManifestSet_SortedNames(t *testing.T) {
	t.Parallel()
	ms, err := NewManifestSet(mustNamedTool(t, "a"))
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, ms.Names())

	ms, err = NewManifestSet(mustNamedTool(t, "z"), mustNamedTool(t, "a"))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "z"}, ms.Names())
}

func TestManifestSet_Manifest(t *testing.T) {
	t.Parallel()
	tool := mustNamedTool(t, "weather")
	ms, err := NewManifestSet(tool)
	require.NoError(t, err)
	m, ok := ms.Manifest("weather")
	require.True(t, ok)
	assert.Equal(t, "weather", m.Name)
	assert.Equal(t, "weather", m.Description)
}

func TestManifestSetFromManifests(t *testing.T) {
	t.Parallel()
	ms, err := ManifestSetFromManifests(
		ToolManifest{Name: "b", Description: "B"},
		ToolManifest{Name: "a", Description: "A"},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, ms.Names())
}

func TestNewManifestSet_DuplicateName(t *testing.T) {
	t.Parallel()
	_, err := NewManifestSet(mustNamedTool(t, "dup"), mustNamedTool(t, "dup"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func TestManifestSetFromManifests_DuplicateName(t *testing.T) {
	t.Parallel()
	_, err := ManifestSetFromManifests(
		ToolManifest{Name: "x", Description: "1"},
		ToolManifest{Name: "x", Description: "2"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func TestManifestSetFromManifests_ClonesRequirements(t *testing.T) {
	t.Parallel()
	req := ToolRequirements{NeedsSession: true}
	ms, err := ManifestSetFromManifests(ToolManifest{
		Name:         "sess",
		Description:  "Session tool",
		Requirements: req,
	})
	require.NoError(t, err)
	m, ok := ms.Manifest("sess")
	require.True(t, ok)
	assert.True(t, m.Requirements.NeedsSession)
	m.Requirements.NeedsSession = false
	m2, _ := ms.Manifest("sess")
	assert.True(t, m2.Requirements.NeedsSession)
}

func TestNewManifestSet_NilTool(t *testing.T) {
	t.Parallel()
	_, err := NewManifestSet(mustNamedTool(t, "a"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil tool")
}

func TestNewManifestSet_EmptyName(t *testing.T) {
	t.Parallel()
	tool := newMiddlewareMinTool("", func(_ context.Context, _ *RunEnv, _ ToolInput, _ func(Chunk) error) error {
		return nil
	})
	_, err := NewManifestSet(tool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest name is required")
}

func TestManifestSetFromManifests_EmptyName(t *testing.T) {
	t.Parallel()
	_, err := ManifestSetFromManifests(ToolManifest{Name: "", Description: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest name is required")
}

func TestRegistry_ManifestSet_MatchesNewManifestSet(t *testing.T) {
	t.Parallel()
	tools := []Tool{
		mustNamedTool(t, "b"),
		mustNamedTool(t, "a"),
	}
	reg := mustBuildRegistry(t, tools)

	fromReg, err := reg.ManifestSet()
	require.NoError(t, err)
	fromNew, err := NewManifestSet(tools...)
	require.NoError(t, err)
	assert.Equal(t, fromNew.Names(), fromReg.Names())
	for _, name := range fromNew.Names() {
		want, ok := fromNew.Manifest(name)
		require.True(t, ok)
		got, ok := fromReg.Manifest(name)
		require.True(t, ok)
		assert.Equal(t, want.Name, got.Name)
		assert.Equal(t, want.Description, got.Description)
	}
}
