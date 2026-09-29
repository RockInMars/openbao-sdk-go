package transitutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestPublicRejectsSkippedMalformedPEM(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	good := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	bad := append([]byte("-----BEGIN PUBLIC KEY-----\n!\n-----END PUBLIC KEY-----\n"), good...)
	if _, _, _, err := Public("ed25519", string(bad)); err == nil {
		t.Fatal("malformed leading public key was silently skipped")
	}
}
