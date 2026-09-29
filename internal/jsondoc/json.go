// Package jsondoc validates untrusted JSON without losing numeric lexemes.
package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var errInvalid = errors.New("invalid JSON document")

const MaxDepth = 64

// Parse rejects duplicate object members, trailing values and excessive nesting.
// It intentionally has no second byte limit: the caller owns its configured
// request/response byte boundary. Returned numbers are json.Number, never float64.
func Parse(raw []byte, objectRoot bool) (any, error) {
	if !utf8.Valid(raw) || !validUnicodeEscapes(raw) {
		return nil, errInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, e := value(d, 0)
	if e != nil {
		return nil, errInvalid
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errInvalid
	}
	if objectRoot {
		if _, ok := v.(map[string]any); !ok {
			return nil, errInvalid
		}
	}
	return v, nil
}
func value(d *json.Decoder, depth int) (any, error) {
	tok, e := d.Token()
	if e != nil {
		return nil, errInvalid
	}
	if delim, ok := tok.(json.Delim); ok {
		if depth >= MaxDepth {
			return nil, errInvalid
		}
		switch delim {
		case '{':
			m := make(map[string]any)
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, errInvalid
				}
				key, ok := k.(string)
				if !ok {
					return nil, errInvalid
				}
				if _, exists := m[key]; exists {
					return nil, errInvalid
				}
				v, e := value(d, depth+1)
				if e != nil {
					return nil, e
				}
				m[key] = v
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return nil, errInvalid
			}
			return m, nil
		case '[':
			a := make([]any, 0)
			for d.More() {
				v, e := value(d, depth+1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return nil, errInvalid
			}
			return a, nil
		default:
			return nil, errInvalid
		}
	}
	switch tok.(type) {
	case string, bool, json.Number, nil:
		return tok, nil
	default:
		return nil, errInvalid
	}
}
func Object(raw []byte) (map[string]any, error) {
	v, e := Parse(raw, true)
	if e != nil {
		return nil, e
	}
	return v.(map[string]any), nil
}
