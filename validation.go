package toolsy

// Validatable is implemented by argument structs that need custom business validation.
// Called after schema validation and unmarshaling.
type Validatable interface {
	Validate() error
}

// schemaValidator validates a JSON-like value (e.g. map[string]any from [json.Unmarshal]).
// Used by both static Extractor and dynamic Tool. *jsonschema.Resolved implements it.
type schemaValidator interface {
	Validate(v any) error
}

// validateAgainstSchema runs Layer 1 validation on already-parsed value v.
// Caller must unmarshal JSON and pass the result; parse errors are reported by the caller (e.g. Extractor.ParseAndValidate or Tool Execute).
func validateAgainstSchema(validate schemaValidator, v any) error {
	if err := validate.Validate(v); err != nil {
		return NewValidationError(err.Error())
	}
	return nil
}

// validateCustom runs Layer 2 (Validatable) if args implements it.
func validateCustom(args any) error {
	if v, ok := args.(Validatable); ok {
		return v.Validate()
	}
	return nil
}

// ValidateContract checks that every name in requiredNames exists in reg.
// Call before starting an agent (fail-fast). Empty requiredNames is a no-op.
// reg must have a valid runtime state (built via [RegistryBuilder]); otherwise returns [*ToolError] with [CodeRegistryNotReady].
func ValidateContract(reg *Registry, requiredNames []string) error {
	if _, err := reg.requireRuntimeState(); err != nil {
		return err
	}
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
		if !reg.Has(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return NewToolsContractMissingError(uniqueRequired, missing)
}
