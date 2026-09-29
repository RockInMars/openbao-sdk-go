package sensitive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestSensitiveFormatting(t *testing.T) {
	secret := NewBytes([]byte("generated-fixture-material"))
	for _, verb := range []string{"%s", "%v", "%+v", "%#v", "%q", "%x", "%X"} {
		if strings.Contains(fmt.Sprintf(verb, secret), "generated-fixture-material") {
			t.Fatal("secret leaked through fmt")
		}
	}
	var out bytes.Buffer
	slog.New(slog.NewJSONHandler(&out, nil)).Info("test", "value", secret)
	if strings.Contains(out.String(), "generated-fixture-material") {
		t.Fatal("secret leaked through slog")
	}
	if _, err := json.Marshal(secret); err == nil {
		t.Fatal("ordinary JSON serialization must fail")
	}
	if secret.String() != "[REDACTED]" || secret.GoString() != "[REDACTED]" {
		t.Fatal("unsafe rendering")
	}
}
func TestSensitiveCopyAndZero(t *testing.T) {
	input := []byte("fixture")
	b := NewBytes(input)
	input[0] = '!'
	copy := b.RevealCopy()
	copy[0] = '?'
	if string(b.RevealCopy()) != "fixture" || b.Len() != 7 {
		t.Fatal("alias escaped")
	}
	b.Zero()
	if b.Len() != 0 {
		t.Fatal("Zero did not clear")
	}
	b.Zero()
	var nilValue *Bytes
	nilValue.Zero()
}
func TestSensitiveConcurrentRead(t *testing.T) {
	b := NewBytes([]byte("local-fixture"))
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			if len(b.RevealCopy()) != b.Len() {
				t.Fatal("copy size")
			}
		})
	}
}
