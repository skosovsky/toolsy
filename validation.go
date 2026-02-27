package toolsy

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validatable is implemented by argument structs that need custom business validation.
// Called after schema validation and unmarshaling.
type Validatable interface {
	Validate() error
}

// validateAgainstSchema runs Layer 1 validation on already-parsed value v.
// Caller must unmarshal JSON once and pass the result; parse errors are handled in one place (Execute path).
func validateAgainstSchema(compiled *jsonschema.Schema, v any) error {
	err := compiled.Validate(v)
	if err == nil {
		return nil
	}
	return &ClientError{Reason: err.Error(), Err: ErrValidation}
}

// validateCustom runs Layer 2 (Validatable) if args implements it.
func validateCustom(args any) error {
	if v, ok := args.(Validatable); ok {
		return v.Validate()
	}
	return nil
}
