package mcp

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// The executable union contract is pinned to the exact protocol schema used by
// this package. Keeping validation schema-driven avoids recreating the large
// sampling and elicitation DTO families as runtime APIs.
//
//go:embed testdata/task34/schema/mcp-2026-07-28.schema.json
var pinnedProtocolSchema []byte

type inputUnionSchemaValidator struct {
	once     sync.Once
	compiled interface{ Validate(any) error }
	err      error
}

//nolint:gochecknoglobals // One immutable lazy-compiled schema is shared by all DTO boundary checks.
var pinnedInputUnionValidator inputUnionSchemaValidator

func validateInputRequests(raw json.RawMessage) error {
	return pinnedInputUnionValidator.validate("inputRequests", raw)
}

func validateInputResponses(raw json.RawMessage) error {
	return pinnedInputUnionValidator.validate("inputResponses", raw)
}

func (v *inputUnionSchemaValidator) validate(field string, raw json.RawMessage) error {
	v.once.Do(v.compile)
	if v.err != nil {
		return v.err
	}
	value, err := jsonschemax.Decode(raw)
	if err != nil {
		return err
	}
	if err = v.compiled.Validate(map[string]any{field: value}); err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	return nil
}

func (v *inputUnionSchemaValidator) compile() {
	decoded, err := jsonschemax.Decode(pinnedProtocolSchema)
	if err != nil {
		v.err = fmt.Errorf("decode pinned MCP schema: %w", err)
		return
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		v.err = errors.New("pinned MCP schema must be an object")
		return
	}
	contract := map[string]any{
		"$schema":              root["$schema"],
		"$defs":                root["$defs"],
		"type":                 schemaTypeObject,
		"additionalProperties": false,
		"properties": map[string]any{
			"inputRequests":  map[string]any{schemaRefKeyword: "#/$defs/InputRequests"},
			"inputResponses": map[string]any{schemaRefKeyword: "#/$defs/InputResponses"},
		},
	}
	v.compiled, v.err = jsonschemax.Compile(contract)
	if v.err != nil {
		v.err = fmt.Errorf("compile pinned MCP input union schema: %w", v.err)
	}
}
