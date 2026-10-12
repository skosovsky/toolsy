package jsonschemax

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

// validateJSON rejects recursive duplicate keys and bounds parser work before
// the schema compiler evaluates references and composition keywords.
//
//nolint:gocognit // The recursive token state machine intentionally centralizes all JSON container invariants.
func referenceValidateJSON(raw []byte, maxDepth, maxNodes int) error {
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

func referenceDecode(raw []byte) (any, error) {
	if err := referenceValidateJSON(raw, maxSchemaDepth, maxSchemaNodes); err != nil {
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

func FuzzPerformanceDecode(f *testing.F) {
	// Arrange: retain the baseline parser as a differential oracle.
	for _, s := range []string{`{}`, `null`, `{"x":1,"\u0078":2}`, `1e999999`, `-0.0`, `[true,false,null]`, `"\ud800"`, `{} {}`, `{"x":`, string([]byte{'"', 0xff, '"'})} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Act.
		want, we := referenceDecode(raw)
		got, ge := Decode(raw)
		// Assert.
		if (we == nil) != (ge == nil) {
			t.Fatalf("acceptance drift %q: baseline=%v candidate=%v", raw, we, ge)
		}
		if we == nil && !reflect.DeepEqual(want, got) {
			t.Fatalf("value drift %q: %v / %v", raw, want, got)
		}
		if we != nil {
			for _, class := range []string{"duplicate", "maximum depth", "maximum node count"} {
				if strings.Contains(we.Error(), class) != strings.Contains(ge.Error(), class) {
					t.Fatalf("error class drift %q: %v / %v", raw, we, ge)
				}
			}
		}
	})
}
func TestPerformanceDecodeLimits(t *testing.T) {
	for _, n := range []int{127, 128, 129} {
		// Arrange.
		raw := []byte(strings.Repeat("[", n) + "0" + strings.Repeat("]", n))
		// Act.
		_, we := referenceDecode(raw)
		_, ge := Decode(raw)
		// Assert.
		if (we == nil) != (ge == nil) {
			t.Fatalf("depth %d: %v / %v", n, we, ge)
		}
	}
	for _, n := range []int{99997, 99998, 99999} {
		// Arrange.
		// One array node plus n+1 values yields limit-1, limit, limit+1.
		raw := []byte("[" + strings.Repeat("0,", n) + "0]")
		// Act.
		_, we := referenceDecode(raw)
		_, ge := Decode(raw)
		// Assert.
		if (we == nil) != (ge == nil) {
			t.Fatalf("nodes %d: %v / %v", n, we, ge)
		}
	}
}
