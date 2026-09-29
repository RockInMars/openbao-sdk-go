package kv

import "testing"

func TestDocumentRejectsLossyUnicode(t *testing.T) {
	doc, err := ParseDocument([]byte(`{"credential":"\ud800"}`))
	defer doc.Zero()
	if err == nil {
		t.Fatal("document accepted a Unicode escape that cannot be decoded losslessly")
	}
}
