package toolsy

// Validatable is implemented by argument structs that need custom business validation.
// Called after schema validation and unmarshaling.
type Validatable interface {
	Validate() error
}

// schemaValidator validates lossless JSON-like values, including [json.Number].
// Used by typed, dynamic and proxy tools with the same exact-number compiler.
type schemaValidator interface {
	Validate(v any) error
}

// validateAgainstSchema runs Layer 1 validation on already-parsed value v.
// Caller must parse lossless JSON and pass the result; parse errors are reported by the caller (e.g. Extractor.ParseAndValidate or Tool Execute).
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
