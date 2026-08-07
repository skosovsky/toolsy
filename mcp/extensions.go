package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ExtensionCodec lets a host bring its own extension type without making that
// extension part of the MCP core protocol package.
type ExtensionCodec interface {
	DecodeExtension(json.RawMessage) (any, error)
	EncodeExtension(any) (json.RawMessage, error)
}

// ExtensionRegistry owns explicitly registered extension identifiers. Merely
// receiving an unknown extension declaration never registers or enables it.
type ExtensionRegistry struct {
	mu     sync.RWMutex
	codecs map[string]ExtensionCodec
}

func (r *ExtensionRegistry) Register(identifier string, codec ExtensionCodec) error {
	if !metaKeyPattern.MatchString(identifier) || !strings.ContainsRune(identifier, '/') {
		return fmt.Errorf("mcp: extension identifier %q must have a namespace prefix", identifier)
	}
	if codec == nil {
		return errors.New("mcp: extension codec must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.codecs == nil {
		r.codecs = make(map[string]ExtensionCodec)
	}
	if _, exists := r.codecs[identifier]; exists {
		return fmt.Errorf("mcp: extension %q is already registered", identifier)
	}
	r.codecs[identifier] = codec
	return nil
}

func (r *ExtensionRegistry) Decode(identifier string, raw json.RawMessage) (any, error) {
	r.mu.RLock()
	codec := r.codecs[identifier]
	r.mu.RUnlock()
	if codec == nil {
		return nil, &UnsupportedFeatureError{Feature: "extension " + identifier}
	}
	if err := validateJSONValue(raw); err != nil {
		return nil, &InvalidPayloadError{Subject: "extension " + identifier, Err: err}
	}
	return codec.DecodeExtension(bytes.Clone(raw))
}

func (r *ExtensionRegistry) Encode(identifier string, value any) (json.RawMessage, error) {
	r.mu.RLock()
	codec := r.codecs[identifier]
	r.mu.RUnlock()
	if codec == nil {
		return nil, &UnsupportedFeatureError{Feature: "extension " + identifier}
	}
	raw, err := codec.EncodeExtension(value)
	if err != nil {
		return nil, err
	}
	if err = validateJSONValue(raw); err != nil {
		return nil, &InvalidPayloadError{Subject: "extension " + identifier, Err: err}
	}
	return bytes.Clone(raw), nil
}
