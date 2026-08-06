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

type denyURLLoader struct{}

func (denyURLLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is disabled", url)
}

// Compile compiles a JSON-like schema value using JSON Schema draft 2020-12
// when the schema does not declare its own dialect.
func Compile(schema any) (*tekuri.Schema, error) {
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
