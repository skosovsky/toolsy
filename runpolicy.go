package toolsy

import (
	"fmt"
	"slices"
)

// RunPolicy describes session-level catalog requirements and call selection.
// Enforcement is applied only by [Session.Execute], not by [Registry.Execute].
// Use [Registry.View] for static capability scoping at registry level.
//
// Semantics:
//   - ForcedTool: every Execute must target this tool name.
//   - AllowedTools: when non-empty, Execute tool name must be in this whitelist.
//   - CatalogRequiredTools: these names must exist in the visible session catalog
//     at construction; this does not require calling them or restrict call selection.
type RunPolicy struct {
	ForcedTool           string
	CatalogRequiredTools []string
	AllowedTools         []string
}

// ValidateRunPolicy checks internal consistency of a run policy.
func ValidateRunPolicy(p RunPolicy) error {
	if err := validateToolNameList("catalog_required_tools", p.CatalogRequiredTools); err != nil {
		return err
	}
	if err := validateToolNameList("allowed_tools", p.AllowedTools); err != nil {
		return err
	}
	if p.ForcedTool == "" && len(p.CatalogRequiredTools) == 0 && len(p.AllowedTools) == 0 {
		return nil
	}
	if p.ForcedTool != "" {
		if err := validateToolName("forced_tool", p.ForcedTool); err != nil {
			return err
		}
	}
	return validateRunPolicyRelations(p)
}

func validateRunPolicyRelations(p RunPolicy) error {
	if len(p.AllowedTools) > 0 && p.ForcedTool != "" && !slices.Contains(p.AllowedTools, p.ForcedTool) {
		return fmt.Errorf("toolsy: forced tool %q is not in allowed tools", p.ForcedTool)
	}
	return nil
}

func cloneRunPolicy(policy RunPolicy) RunPolicy {
	policy.AllowedTools = slices.Clone(policy.AllowedTools)
	policy.CatalogRequiredTools = slices.Clone(policy.CatalogRequiredTools)
	return policy
}

func validateRunPolicyCatalog(policy RunPolicy, names []string) error {
	var missing []string
	for _, name := range policy.CatalogRequiredTools {
		if !slices.Contains(names, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return NewToolsContractMissingError(policy.CatalogRequiredTools, missing)
	}
	return nil
}

func validateToolName(field, name string) error {
	if name == "" {
		return fmt.Errorf("toolsy: %s must not be empty", field)
	}
	return nil
}

func validateToolNameList(field string, names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("toolsy: %s must not contain empty tool names", field)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("toolsy: %s contains duplicate %q", field, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// RegistryProvider resolves a registry lazily after builder configuration (validator late binding).
type RegistryProvider func() (*Registry, error)
