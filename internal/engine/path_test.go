package engine

import (
	"strings"
	"testing"
)

func TestPathValidation(t *testing.T) {
	for _, s := range []string{"", "../x", "a//b", "a/./b", "a/../b", "a/", "/a", "a\\b", "a%2fb", "a?x", "a#f", "a\x00b", "a\nb", "a.", "中文"} {
		if ValidatePath(s) == nil {
			t.Fatal("unsafe path accepted")
		}
	}
	for _, s := range []string{"a", "a-b/c_d", "a.b/c-1"} {
		if ValidatePath(s) != nil {
			t.Fatal("valid path rejected")
		}
	}
	if ValidateSegment("a/b") == nil || ValidateSegment(".") == nil {
		t.Fatal("segment escape")
	}
}
func FuzzPath(f *testing.F) {
	for _, s := range []string{"a/b", "../a", "a%2fb", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8192 {
			return
		}
		if ValidatePath(s) == nil {
			if strings.Contains(s, "//") || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "%?\\#\n\r") {
				t.Fatal("path escaped")
			}
		}
	})
}
