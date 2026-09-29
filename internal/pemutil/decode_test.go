package pemutil

import (
	"bytes"
	"encoding/pem"
	"testing"
)

func TestDecodeStartsAtFirstBlock(t *testing.T) {
	good := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("fixture DER")})
	malformed := []byte("-----BEGIN CERTIFICATE-----\n!\n-----END CERTIFICATE-----\n")
	for _, bad := range [][]byte{nil, []byte("garbage"), malformed, append(bytes.Clone(malformed), good...), append([]byte("\n"), good...), append([]byte("junk\n"), good...)} {
		b, rest := Decode(bad)
		if b != nil || !bytes.Equal(rest, bad) {
			t.Fatal("invalid prefix not rejected without consuming bytes")
		}
	}
	for _, raw := range [][]byte{good, bytes.ReplaceAll(good, []byte("\n"), []byte("\r\n"))} {
		b, rest := Decode(raw)
		if b == nil || b.Type != "CERTIFICATE" || string(b.Bytes) != "fixture DER" || len(rest) != 0 {
			t.Fatal("valid PEM rejected")
		}
	}
	raw := append(bytes.Clone(good), good...)
	b, rest := Decode(raw)
	if b == nil || !bytes.Equal(rest, good) {
		t.Fatal("valid next block lost")
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte("-----BEGIN CERTIFICATE-----\n!\n-----END CERTIFICATE-----\n"))
	f.Add(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("fixture")}))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<20 {
			return
		}
		b, rest := Decode(raw)
		if b == nil {
			if !bytes.Equal(rest, raw) {
				t.Fatal("failure consumed input")
			}
		} else if len(rest) >= len(raw) || !bytes.HasPrefix(raw, []byte("-----BEGIN ")) || bytes.Count(raw[:len(raw)-len(rest)], []byte("-----BEGIN ")) != 1 {
			t.Fatal("decoder skipped malformed framing")
		}
	})
}
