package kv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/RockInMars/openbao-sdk-go/internal/jsondoc"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"io"
	"log/slog"
)

// Document is a validated JSON object. Ordinary marshaling is deliberately denied.
// Value copies share a best-effort erasure handle, just like sensitive.Bytes.
type Document struct{ raw sensitive.Bytes }

// NewDocument encodes a JSON object into an owned secret document. The caller
// retains ownership of value, including any plaintext strings or byte slices.
func NewDocument(value any) (Document, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return Document{}, errors.New("cannot encode secret document")
	}
	defer clear(raw)
	return ParseDocument(raw)
}

// ParseDocument validates and copies a JSON object. It does not clear raw.
func ParseDocument(raw []byte) (Document, error) {
	if _, e := jsondoc.Object(raw); e != nil {
		return Document{}, errors.New("invalid secret JSON object")
	}
	return Document{raw: sensitive.NewBytes(raw)}, nil
}

// Decode reveals the document into dst. The caller must manage decoded secrets;
// clearing the document does not erase strings, slices, or values stored in dst.
func (d Document) Decode(dst any) error {
	raw := d.RevealJSON()
	defer clear(raw)
	if len(raw) == 0 {
		return errors.New("empty secret document")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return errors.New("cannot decode secret document")
	}
	return nil
}

// RevealJSON returns an independent plaintext JSON copy; clear it after use.
func (d Document) RevealJSON() []byte { return d.raw.RevealCopy() }

// Zero clears this document and its value copies, which share an erasure handle.
func (d *Document) Zero() {
	if d != nil {
		d.raw.Zero()
	}
}
func (d Document) String() string                    { return "[REDACTED]" }
func (d Document) GoString() string                  { return "[REDACTED]" }
func (d Document) Format(state fmt.State, verb rune) { _, _ = io.WriteString(state, "[REDACTED]") }
func (d Document) LogValue() slog.Value              { return slog.StringValue("[REDACTED]") }
func (d Document) MarshalJSON() ([]byte, error) {
	return nil, errors.New("secret document requires explicit reveal")
}
