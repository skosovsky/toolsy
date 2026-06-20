package toolsy

import (
	"fmt"
	"slices"
)

// RunPolicy describes session-level tool choice constraints for LLM adapters.
// Enforcement is applied only by [Session.Execute], not by [Registry.Execute].
// Use [Registry.View] for static capability scoping at registry level.
//
// Semantics:
//   - ForcedTool: every Execute must target this tool name.
//   - AllowedTools: when non-empty, Execute tool name must be in this whitelist.
//   - RequiredTools: when AllowedTools is empty and RequiredTools is non-empty,
//     Execute tool name must be in RequiredTools (whitelist for adapters that
//     declare a required tool set without an explicit allowed list).
type RunPolicy struct {
	ForcedTool    string
	RequiredTools []string
	AllowedTools  []string
}

// ValidateRunPolicy checks internal consistency of a run policy.
func ValidateRunPolicy(p RunPolicy) error {
	if err := validateToolNameList("required_tools", p.RequiredTools); err != nil {
		return err
	}
	if err := validateToolNameList("allowed_tools", p.AllowedTools); err != nil {
		return err
	}
	if p.ForcedTool == "" && len(p.RequiredTools) == 0 && len(p.AllowedTools) == 0 {
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
	if len(p.RequiredTools) > 0 && p.ForcedTool != "" && !slices.Contains(p.RequiredTools, p.ForcedTool) {
		return fmt.Errorf("toolsy: forced tool %q is not in required tools", p.ForcedTool)
	}
	if len(p.RequiredTools) > 0 && len(p.AllowedTools) > 0 {
		for _, name := range p.RequiredTools {
			if !slices.Contains(p.AllowedTools, name) {
				return fmt.Errorf("toolsy: required tool %q is not in allowed tools", name)
			}
		}
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
