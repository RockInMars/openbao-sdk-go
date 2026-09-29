package jsondoc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictTree(t *testing.T) {
	for _, s := range []string{`{"n":9007199254740993,"d":1.000000000000000001,"a":[true,false,null,{"ok":"x"}]}`, `{}`, `{"nested":{"array":[]}}`} {
		v, e := Object([]byte(s))
		if e != nil || v == nil {
			t.Fatal("valid object rejected")
		}
	}
	v, e := Object([]byte(`{"n":9007199254740993}`))
	if e != nil || v["n"].(json.Number).String() != "9007199254740993" {
		t.Fatal("numeric lexeme lost")
	}
	for _, s := range []string{"", `null`, `[]`, `1`, `true`, `"x"`, `{} {}`, `{"a":1,"\u0061":2}`, `{"a":[{"x":1,"x":2}]}`, `{"a":}`, `{"a" 1}`, `{1:2}`, `[1,]`, `{"a":[]`, "{\"a\":\"\xff\"}", strings.Repeat(`{"a":`, 65) + "0" + strings.Repeat("}", 65)} {
		if _, e := Object([]byte(s)); e == nil {
			t.Fatal("malformed/non-object tree accepted")
		}
	}
	for _, s := range []string{`null`, `[1,true,"ok"]`, `1`, `false`} {
		if _, e := Parse([]byte(s), false); e != nil {
			t.Fatal("non-object parse mode broken")
		}
	}
}
func FuzzJSON(f *testing.F) {
	for _, s := range []string{`{}`, `{"a":1,"a":2}`, `[1]`, `{"a":[false,null]}`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			return
		}
		v, e := Parse([]byte(s), false)
		if e == nil {
			b, e := json.Marshal(v)
			if e != nil {
				t.Fatal("valid tree not serializable")
			}
			if _, e = Parse(b, false); e != nil {
				t.Fatal("valid tree did not roundtrip")
			}
		}
	})
}
