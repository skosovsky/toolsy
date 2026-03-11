package openapi

import (
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
)

// matchMethod returns true if method is in AllowedMethods (or AllowedMethods is empty).
func matchMethod(_ *openapi3.Operation, method string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	return slices.Contains(allowed, method)
}

// matchTags returns true if the operation has at least one tag in AllowedTags,
// or if AllowedTags is empty (then any operation passes).
// If the operation has no tags, it passes only when AllowedTags is empty.
func matchTags(op *openapi3.Operation, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	if len(op.Tags) == 0 {
		return false
	}
	for _, t := range op.Tags {
		if slices.Contains(allowed, t) {
			return true
		}
	}
	return false
}

// includeOperation returns true if the operation passes method and tag filters.
func includeOperation(op *openapi3.Operation, method string, opts *Options) bool {
	return op != nil && matchMethod(op, method, opts.AllowedMethods) && matchTags(op, opts.AllowedTags)
}
