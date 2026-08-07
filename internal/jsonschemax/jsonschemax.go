// Package jsonschemax provides lossless JSON Schema compilation for schemas
// received over wire protocols. Numeric keywords are kept as [json.Number] so
// constraints beyond IEEE-754 precision remain exact.
package jsonschemax

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	tekuri "github.com/santhosh-tekuri/jsonschema/v6"
)

// A hierarchical synthetic base is required so relative nested $id values
// resolve distinctly. No URL loader is installed, so this never enables I/O.
const schemaResource = "https://toolsy.invalid/schemas/wire-schema"

const (
	maxSchemaDepth = 128
	maxSchemaNodes = 100_000
)

type denyURLLoader struct{}

func (denyURLLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is disabled", url)
}

// Compile compiles a JSON-like schema value using JSON Schema draft 2020-12
// when the schema does not declare its own dialect.
func Compile(schema any) (*tekuri.Schema, error) {
	if err := checkComplexity(schema, maxSchemaDepth, maxSchemaNodes); err != nil {
		return nil, err
	}
	compiler := tekuri.NewCompiler()
	compiler.DefaultDraft(tekuri.Draft2020)
	compiler.UseLoader(denyURLLoader{})
	if err := compiler.AddResource(schemaResource, schema); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaResource)
}

// Decode parses exactly one JSON value while preserving numeric precision.
func Decode(raw []byte) (any, error) {
	if err := validateJSON(raw, maxSchemaDepth, maxSchemaNodes); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON input must contain exactly one value")
	}
	return value, nil
}

func checkComplexity(value any, maxDepth, maxNodes int) error {
	nodes := 0
	var walk func(any, int) error
	walk = func(current any, depth int) error {
		if depth > maxDepth {
			return fmt.Errorf("JSON schema exceeds maximum depth %d", maxDepth)
		}
		nodes++
		if nodes > maxNodes {
			return fmt.Errorf("JSON schema exceeds maximum node count %d", maxNodes)
		}
		switch typed := current.(type) {
		case map[string]any:
			for _, nested := range typed {
				if err := walk(nested, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, nested := range typed {
				if err := walk(nested, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value, 0)
}

// validateJSON rejects recursive duplicate keys and bounds parser work before
// the schema compiler evaluates references and composition keywords.
//
//nolint:gocognit // The recursive token state machine intentionally centralizes all JSON container invariants.
func validateJSON(raw []byte, maxDepth, maxNodes int) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	nodes := 0
	var walk func(int) error
	walk = func(depth int) error {
		if depth > maxDepth {
			return fmt.Errorf("JSON exceeds maximum depth %d", maxDepth)
		}
		nodes++
		if nodes > maxNodes {
			return fmt.Errorf("JSON exceeds maximum node count %d", maxNodes)
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key must be a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim('}') {
				return errors.New("invalid JSON object")
			}
		case '[':
			for decoder.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, endErr := decoder.Token()
			if endErr != nil || end != json.Delim(']') {
				return errors.New("invalid JSON array")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("JSON input must contain exactly one value")
	}
	return nil
}
