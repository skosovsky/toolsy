package toolsy

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ManifestSet is a declarative collection of tool manifests without executable registry state.
type ManifestSet struct {
	byName map[string]ToolManifest
}

// NewManifestSet builds a manifest set from tools (Manifest() only; no Build required).
func NewManifestSet(tools ...Tool) (ManifestSet, error) {
	ms := ManifestSet{byName: make(map[string]ToolManifest, len(tools))}
	for _, tool := range tools {
		if err := addToolToManifestSet(&ms, tool); err != nil {
			return ManifestSet{}, err
		}
	}
	return ms, nil
}

// ManifestSetFromManifests builds a set from raw manifests.
func ManifestSetFromManifests(manifests ...ToolManifest) (ManifestSet, error) {
	ms := ManifestSet{byName: make(map[string]ToolManifest, len(manifests))}
	for _, m := range manifests {
		cloned := cloneToolManifest(m)
		if cloned.Name == "" {
			return ManifestSet{}, errors.New("toolsy: tool manifest name is required")
		}
		if _, exists := ms.byName[cloned.Name]; exists {
			return ManifestSet{}, fmt.Errorf("toolsy: duplicate tool name %q in manifest set", cloned.Name)
		}
		ms.byName[cloned.Name] = cloned
	}
	return ms, nil
}

func manifestSetFromToolMap(tools map[string]Tool) (ManifestSet, error) {
	if len(tools) == 0 {
		return ManifestSet{}, nil
	}
	ms := ManifestSet{byName: make(map[string]ToolManifest, len(tools))}
	for _, tool := range tools {
		if err := addToolToManifestSet(&ms, tool); err != nil {
			return ManifestSet{}, err
		}
	}
	return ms, nil
}

func addToolToManifestSet(ms *ManifestSet, tool Tool) error {
	if tool == nil {
		return errors.New("toolsy: nil tool in manifest set")
	}
	m := cloneToolManifest(tool.Manifest())
	if m.Name == "" {
		return errors.New("toolsy: tool manifest name is required")
	}
	if _, exists := ms.byName[m.Name]; exists {
		return fmt.Errorf("toolsy: duplicate tool name %q in manifest set", m.Name)
	}
	ms.byName[m.Name] = m
	return nil
}

func cloneToolManifest(m ToolManifest) ToolManifest {
	out := m
	out.Tags = append([]string(nil), m.Tags...)
	out.Parameters = maps.Clone(m.Parameters)
	out.OutputSchema = maps.Clone(m.OutputSchema)
	out.Requirements = cloneRequirements(m.Requirements)
	return out
}

// Has reports whether name is present in the set.
func (m ManifestSet) Has(name string) bool {
	if m.byName == nil {
		return false
	}
	_, ok := m.byName[name]
	return ok
}

// Names returns sorted tool names in the set.
func (m ManifestSet) Names() []string {
	if len(m.byName) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.byName))
	for name := range m.byName {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Manifest returns a defensive copy of the manifest for name and whether it exists.
func (m ManifestSet) Manifest(name string) (ToolManifest, bool) {
	if m.byName == nil {
		return ToolManifest{}, false
	}
	manifest, ok := m.byName[name]
	if !ok {
		return ToolManifest{}, false
	}
	return cloneToolManifest(manifest), true
}

// ValidateManifestContract checks that every name in requiredNames exists in ms.
// Empty requiredNames is a no-op.
func ValidateManifestContract(ms ManifestSet, requiredNames []string) error {
	if len(requiredNames) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(requiredNames))
	uniqueRequired := make([]string, 0, len(requiredNames))
	var missing []string
	for _, name := range requiredNames {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		uniqueRequired = append(uniqueRequired, name)
		if !ms.Has(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return NewToolsContractMissingError(uniqueRequired, missing)
}
