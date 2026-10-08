package toolsygen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

// decodeYAMLManifest preserves JSON number precision and bounds the syntax tree
// before converting it. YAML aliases and non-JSON tags are outside the subset.
func decodeYAMLManifest(data []byte) (any, error) {
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("manifest YAML must contain exactly one document")
	}
	nodes := 0
	return yamlJSONValue(&node, 0, &nodes)
}

func yamlJSONValue(node *yaml.Node, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 128 || *nodes > 100_000 {
		return nil, errors.New("manifest YAML exceeds depth/node limits")
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return nil, errors.New("manifest YAML must contain one document")
		}
		return yamlJSONValue(node.Content[0], depth+1, nodes)
	case yaml.MappingNode:
		return yamlJSONMapping(node, depth, nodes)
	case yaml.SequenceNode:
		out := make([]any, len(node.Content))
		for i, child := range node.Content {
			value, err := yamlJSONValue(child, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case yaml.ScalarNode:
		return yamlJSONScalar(node)
	case yaml.AliasNode:
		return nil, fmt.Errorf("manifest YAML aliases and node kind %d are unsupported", node.Kind)
	default:
		return nil, fmt.Errorf("manifest YAML aliases and node kind %d are unsupported", node.Kind)
	}
}

func yamlJSONMapping(node *yaml.Node, depth int, nodes *int) (any, error) {
	out := make(map[string]any, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, errors.New("manifest YAML mapping keys must be strings")
		}
		if _, exists := out[key.Value]; exists {
			return nil, fmt.Errorf("manifest YAML duplicate key %q", key.Value)
		}
		value, err := yamlJSONValue(node.Content[i+1], depth+1, nodes)
		if err != nil {
			return nil, err
		}
		out[key.Value] = value
	}
	return out, nil
}

func yamlJSONScalar(node *yaml.Node) (any, error) {
	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!null":
		return nil, nil //nolint:nilnil // JSON null is a valid scalar representation.
	case "!!bool":
		return strings.EqualFold(node.Value, "true"), nil
	case "!!int":
		raw := strings.ReplaceAll(node.Value, "_", "")
		n, ok := new(big.Int).SetString(raw, 0)
		if !ok {
			return nil, fmt.Errorf("unsupported YAML integer %q", node.Value)
		}
		return json.Number(n.String()), nil
	case "!!float":
		raw := strings.ReplaceAll(node.Value, "_", "")
		raw = strings.TrimPrefix(raw, "+")
		if strings.HasPrefix(raw, ".") {
			raw = "0" + raw
		}
		if strings.HasPrefix(raw, "-.") {
			raw = "-0" + raw[1:]
		}
		if strings.HasSuffix(raw, ".") {
			raw += "0"
		}
		value, err := jsonschemax.Decode([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("unsupported YAML JSON number %q: %w", node.Value, err)
		}
		if _, ok := value.(json.Number); !ok {
			return nil, fmt.Errorf("unsupported YAML number %q", node.Value)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported YAML tag %q", node.Tag)
	}
}
