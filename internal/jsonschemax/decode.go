package jsonschemax

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// decodeValid builds one lossless tree after the standard library's allocation-free
// syntax scan. Invalid documents use the original token walk to preserve rejection
// precedence (duplicates and resource limits can precede a later syntax error).
func decodeValid(raw []byte) (any, error) {
	parser := valueParser{raw: raw, pos: 0, nodes: 0}
	return parser.value(0)
}

type valueParser struct {
	raw   []byte
	pos   int
	nodes int
}

func (p *valueParser) space() {
	for p.pos < len(p.raw) {
		switch p.raw[p.pos] {
		case ' ', '\n', '\r', '\t':
			p.pos++
		default:
			return
		}
	}
}

func (p *valueParser) text() string {
	start := p.pos + 1
	p.pos++
	escaped := false
	for p.raw[p.pos] != '"' {
		if p.raw[p.pos] == '\\' {
			escaped = true
			p.pos++
		}
		p.pos++
	}
	end := p.pos
	p.pos++
	if !escaped && utf8.Valid(p.raw[start:end]) {
		return string(p.raw[start:end])
	}
	var value string
	// json.Valid already checked escapes; Unmarshal retains Go's UTF-8/surrogate handling.
	_ = json.Unmarshal(p.raw[start-1:p.pos], &value)
	return value
}

//nolint:nilnil // JSON null is a valid nil value.
func (p *valueParser) value(depth int) (any, error) {
	if depth > maxSchemaDepth {
		return nil, fmt.Errorf("JSON exceeds maximum depth %d", maxSchemaDepth)
	}
	p.nodes++
	if p.nodes > maxSchemaNodes {
		return nil, fmt.Errorf("JSON exceeds maximum node count %d", maxSchemaNodes)
	}
	p.space()
	switch p.raw[p.pos] {
	case '{':
		return p.object(depth)
	case '[':
		return p.array(depth)
	case '"':
		return p.text(), nil
	case 't':
		p.pos += 4
		return true, nil
	case 'f':
		p.pos += 5
		return false, nil
	case 'n':
		p.pos += 4
		return nil, nil
	default:
		start := p.pos
		for p.pos < len(p.raw) {
			switch p.raw[p.pos] {
			case ',', ']', '}', ' ', '\n', '\r', '\t':
				return json.Number(string(p.raw[start:p.pos])), nil
			default:
				p.pos++
			}
		}
		return json.Number(string(p.raw[start:p.pos])), nil
	}
}

func (p *valueParser) object(depth int) (any, error) {
	out := make(map[string]any)
	p.pos++
	p.space()
	if p.raw[p.pos] == '}' {
		p.pos++
		return out, nil
	}
	for {
		key := p.text()
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate JSON object key %q", key)
		}
		p.space()
		p.pos++
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out[key] = value
		p.space()
		if p.raw[p.pos] == '}' {
			p.pos++
			return out, nil
		}
		p.pos++
		p.space()
	}
}

func (p *valueParser) array(depth int) (any, error) {
	out := []any{}
	p.pos++
	p.space()
	if p.raw[p.pos] == ']' {
		p.pos++
		return out, nil
	}
	for {
		value, err := p.value(depth + 1)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		p.space()
		if p.raw[p.pos] == ']' {
			p.pos++
			return out, nil
		}
		p.pos++
	}
}
