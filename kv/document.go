package kv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"git.example.com/infra/openbao-sdk-go/internal/jsondoc"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"io"
	"log/slog"
)

// Document is a validated JSON object. Ordinary marshaling is deliberately denied.
// Value copies share a best-effort erasure handle, just like sensitive.Bytes.
type Document struct{ raw sensitive.Bytes }

func NewDocument(value any) (Document, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return Document{}, errors.New("cannot encode secret document")
	}
	defer clear(raw)
	return ParseDocument(raw)
}
func ParseDocument(raw []byte) (Document, error) {
	if _, e := jsondoc.Object(raw); e != nil {
		return Document{}, errors.New("invalid secret JSON object")
	}
	return Document{raw: sensitive.NewBytes(raw)}, nil
}
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
func (d Document) RevealJSON() []byte { return d.raw.RevealCopy() }
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
