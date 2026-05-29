package toolsy

import (
	"context"
	"errors"
	"fmt"
)

type lazyRegistryValidator struct {
	provider RegistryProvider
	delegate func(*Registry) Validator
}

func (v *lazyRegistryValidator) Validate(ctx context.Context, toolName string, argsJSON string) error {
	if v.provider == nil {
		return errors.New("toolsy: lazy validator provider is nil")
	}
	if v.delegate == nil {
		return errors.New("toolsy: lazy validator delegate is nil")
	}
	reg, err := v.provider()
	if err != nil {
		return fmt.Errorf("toolsy: lazy validator resolve registry: %w", err)
	}
	if reg == nil {
		return errors.New("toolsy: lazy validator provider returned nil registry")
	}
	validator := v.delegate(reg)
	if validator == nil {
		return nil
	}
	if _, isLazy := validator.(*lazyRegistryValidator); isLazy {
		return errors.New("toolsy: lazy validator delegate must not return another lazy validator")
	}
	return validator.Validate(ctx, toolName, argsJSON)
}

// WithValidatorFromRegistry binds validation to a lazily resolved registry.
// delegate must be non-nil and must not return another lazy validator.
func WithValidatorFromRegistry(provider RegistryProvider, delegate func(*Registry) Validator) RegistryOption {
	return func(o *registryOptions) {
		o.validator = &lazyRegistryValidator{
			provider: provider,
			delegate: delegate,
		}
	}
}
