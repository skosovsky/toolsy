package toolsy

// Validatable is implemented by argument structs that need custom business validation.
// Called after schema validation and unmarshaling.
type Validatable interface {
	Validate() error
}

// schemaValidator validates a JSON-like value (e.g. map[string]any from json.Unmarshal).
// Used by both static Extractor and dynamic Tool. *jsonschema.Resolved implements it.
type schemaValidator interface {
	Validate(v any) error
}

// validateAgainstSchema runs Layer 1 validation on already-parsed value v.
// Caller must unmarshal JSON and pass the result; parse errors are reported by the caller (e.g. Extractor.ParseAndValidate or Tool Execute).
func validateAgainstSchema(validate schemaValidator, v any) error {
	if err := validate.Validate(v); err != nil {
		return &ClientError{Reason: err.Error(), Err: ErrValidation}
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
