package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const (
	snapshotDigestDomain     = "github.com/skosovsky/toolsy/mcp:snapshot:v1"
	maxSnapshotEncodingBytes = 8 << 20
)

// Snapshot is the closed set of cacheable MCP discovery and feature snapshots.
// Implementations are provided by this package so unrelated values cannot be
// accidentally hashed as protocol snapshots.
type Snapshot interface {
	mcpSnapshotDomain() string
}

// SnapshotDigest is the SHA-256 identity of a validated, canonically encoded
// snapshot. The digest includes the snapshot type and all wire fields,
// including ttlMs and cacheScope.
type SnapshotDigest [sha256.Size]byte

func (d SnapshotDigest) String() string { return hex.EncodeToString(d[:]) }

func (DiscoverResult) mcpSnapshotDomain() string              { return MethodServerDiscover }
func (ToolsListResult) mcpSnapshotDomain() string             { return MethodToolsList }
func (ResourcesListResult) mcpSnapshotDomain() string         { return MethodResourcesList }
func (ResourceTemplatesListResult) mcpSnapshotDomain() string { return MethodResourceTemplatesList }
func (ResourcesReadResult) mcpSnapshotDomain() string         { return MethodResourcesRead }
func (PromptsListResult) mcpSnapshotDomain() string           { return MethodPromptsList }

// ComputeSnapshotDigest validates snapshot through its strict wire encoder,
// canonicalizes objects and exact JSON numbers, and returns a type-separated
// deterministic digest. Pointer and value forms produce the same digest.
func ComputeSnapshotDigest(snapshot Snapshot) (SnapshotDigest, error) {
	var zero SnapshotDigest
	if snapshot == nil {
		return zero, errors.New("mcp: snapshot must not be nil")
	}
	value := reflect.ValueOf(snapshot)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return zero, errors.New("mcp: snapshot must not be a nil pointer")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return zero, fmt.Errorf("mcp: validate snapshot: %w", err)
	}
	if len(raw) > maxSnapshotEncodingBytes {
		return zero, fmt.Errorf(
			"mcp: snapshot encoding exceeds maximum size %d bytes",
			maxSnapshotEncodingBytes,
		)
	}
	canonical, err := canonicalSnapshotEncoding(raw)
	if err != nil {
		return zero, fmt.Errorf("mcp: canonicalize snapshot: %w", err)
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(snapshotDigestDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(snapshot.mcpSnapshotDomain()))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(canonical)
	var digest SnapshotDigest
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func canonicalSnapshotEncoding(raw []byte) ([]byte, error) {
	value, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err = appendCanonicalSnapshotValue(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func appendCanonicalSnapshotValue(out *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		return appendCanonicalSnapshotString(out, typed)
	case json.Number:
		canonical, ok := canonicalJSONNumber(typed.String())
		if !ok {
			return fmt.Errorf("invalid JSON number %q", typed)
		}
		out.WriteString(canonical)
	case []any:
		return appendCanonicalSnapshotArray(out, typed)
	case map[string]any:
		return appendCanonicalSnapshotObject(out, typed)
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
	return nil
}

func appendCanonicalSnapshotString(out *bytes.Buffer, value string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	out.Write(encoded)
	return nil
}

func appendCanonicalSnapshotArray(out *bytes.Buffer, values []any) error {
	out.WriteByte('[')
	for index, value := range values {
		if index != 0 {
			out.WriteByte(',')
		}
		if err := appendCanonicalSnapshotValue(out, value); err != nil {
			return err
		}
	}
	out.WriteByte(']')
	return nil
}

func appendCanonicalSnapshotObject(out *bytes.Buffer, values map[string]any) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out.WriteByte('{')
	for index, key := range keys {
		if index != 0 {
			out.WriteByte(',')
		}
		if err := appendCanonicalSnapshotString(out, key); err != nil {
			return err
		}
		out.WriteByte(':')
		if err := appendCanonicalSnapshotValue(out, values[key]); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}
