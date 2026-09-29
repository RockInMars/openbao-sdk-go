package jsondoc

import (
	"testing"
)

func TestJSONRejectsUnpairedSurrogateEscapes(t *testing.T) {
	for _, raw := range []string{
		`{"value":"\ud800"}`, `{"value":"\udfff"}`,
		`{"value":"\ud800\u0041"}`, `{"value":"\ud800\ud800"}`,
		`{"value":"\udc00\ud800"}`, `{"\ud800":1}`,
		`{"nested":["\ud800\\udc00"]}`,
	} {
		if _, err := Object([]byte(raw)); err == nil {
			t.Fatal("unpaired surrogate accepted and would be replaced during decoding")
		}
	}
}

func TestJSONPreservesValidUnicodeEscapes(t *testing.T) {
	cases := map[string]string{
		`{"value":"\ud83d\ude80"}`:             "🚀",
		`{"value":"\uD83D\uDE80"}`:             "🚀",
		`{"value":"\\ud800"}`:                  `\ud800`,
		`{"value":"\ufffd"}`:                   "�",
		`{"value":"\u0000\/\"\\"}`:             "\x00/\"\\",
		`{"value":"中文"}`:                       "中文",
		`{"value":"\ud800\udc00\udbff\udfff"}`: "\U00010000\U0010ffff",
	}
	for raw, want := range cases {
		got, err := Object([]byte(raw))
		if err != nil || got["value"] != want {
			t.Fatal("valid Unicode or literal escaped backslash was corrupted")
		}
	}
}
