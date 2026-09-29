package kv

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDocumentExactNumber(t *testing.T) {
	d, err := ParseDocument([]byte(`{"n":9007199254740993,"array":[true,null,{"x":1}],"decimal":1.234567890123456789}`))
	if err != nil {
		t.Fatal("valid document rejected")
	}
	var v map[string]any
	if err = d.Decode(&v); err != nil {
		t.Fatal("decode failed")
	}
	if v["n"].(json.Number).String() != "9007199254740993" {
		t.Fatal("integer lost precision")
	}
	if !strings.Contains(string(d.RevealJSON()), "1.234567890123456789") {
		t.Fatal("decimal lost precision")
	}
	if _, err = json.Marshal(d); err == nil {
		t.Fatal("ordinary serialization accepted")
	}
	if strings.Contains(fmt.Sprintf("%s%v%+v%#v%x", d, d, d, d, d), "9007199254740993") {
		t.Fatal("format leaked")
	}
	for _, s := range []string{`null`, `[]`, `1`, `{"a":1,"a":2}`, `{"a":{"x":1,"x":2}}`, `{} {}`, `{"a":NaN}`, strings.Repeat(`{"x":`, 65) + `null` + strings.Repeat(`}`, 65)} {
		if _, err := ParseDocument([]byte(s)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
func TestDocumentIndependentCopy(t *testing.T) {
	raw := []byte(`{"a":123}`)
	d, err := ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	c := d.RevealJSON()
	c[0] = '!'
	var x struct {
		A int `json:"a"`
	}
	if d.Decode(&x) != nil || x.A != 123 {
		t.Fatal("mutable alias")
	}
	d.Zero()
	if len(d.RevealJSON()) != 0 {
		t.Fatal("zero")
	}
	if d.Decode(&x) == nil {
		t.Fatal("zero value decoded")
	}
	var p *Document
	p.Zero()
}
func TestDocumentConstructAndDecode(t *testing.T) {
	d, err := NewDocument(map[string]any{"n": int64(9007199254740993)})
	if err != nil {
		t.Fatal("new failed")
	}
	var x map[string]any
	if d.Decode(&x) != nil || x["n"].(json.Number).String() != "9007199254740993" {
		t.Fatal("lossy constructor")
	}
	if _, err := NewDocument(make(chan int)); err == nil {
		t.Fatal("unsupported type")
	}
	if _, err := NewDocument(nil); err == nil {
		t.Fatal("nil root")
	}
	if err := d.Decode(nil); err == nil {
		t.Fatal("nil destination")
	}
}
func FuzzDocument(f *testing.F) {
	for _, s := range []string{`{}`, `{"a":1}`, `null`, `{"a":1,"a":2}`, `{"value":"\ud800"}`, `{"value":"\ud83d\ude80"}`, `{"value":"\\ud800"}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		d, e := ParseDocument(b)
		if e != nil {
			return
		}
		var x map[string]any
		if d.Decode(&x) != nil {
			t.Fatal("inconsistent parser")
		}
		if _, e := ParseDocument(d.RevealJSON()); e != nil {
			t.Fatal("roundtrip invalid")
		}
	})
}
