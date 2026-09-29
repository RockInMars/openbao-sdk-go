package transitutil

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"git.example.com/infra/openbao-sdk-go/transit"
	"strings"
	"testing"
)

func TestStrictWrappedEncoding(t *testing.T) {
	for _, s := range []string{"", "vault:v0:AA==", "vault:v01:AA==", "vault:v-1:AA==", "other:v1:AA==", "vault:1:AA==", "vault:v1:", "vault:v1:AA", "vault:v1:AB==", "vault:v1:AA==\n", "vault:v1:AA==:extra", "vault:v99999999999999999999:AA=="} {
		if _, _, e := Unwrap(s, 4096); e == nil {
			t.Fatal("non-canonical envelope accepted")
		}
	}
	v, b, e := Unwrap("vault:v2:AA==", 4096)
	if e != nil || v != 2 || !bytes.Equal(b, []byte{0}) {
		t.Fatal("valid envelope rejected")
	}
	if _, _, e = Unwrap("vault:v1:AA==", 1); e == nil {
		t.Fatal("size bound ignored")
	}
}
func TestPublicNormalizationAndSignatureProfiles(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rs, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, ed, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("fixture signature input")
	sum := sha256.Sum256(msg)
	for _, x := range []struct {
		kind string
		p    transit.Profile
		k    crypto.Signer
	}{{"ecdsa-p256", transit.ECDSAP256SHA256ASN1, ec}, {"rsa-2048", transit.RSAPSSSHA256, rs}, {"ed25519", transit.Ed25519Message, ed}} {
		der, _ := x509.MarshalPKIXPublicKey(x.k.Public())
		text := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
		texts := []string{text}
		if x.kind == "ed25519" {
			texts = append(texts, base64.StdEncoding.EncodeToString(ed.Public().(ed25519.PublicKey)))
		}
		for _, s := range texts {
			pub, normalized, out, e := Public(x.kind, s)
			if e != nil || !bytes.Equal(normalized, der) || len(out) == 0 || !Compatible(x.kind, x.p) {
				t.Fatal("normalization failed")
			}
			var sig []byte
			switch k := x.k.(type) {
			case *ecdsa.PrivateKey:
				sig, _ = ecdsa.SignASN1(rand.Reader, k, sum[:])
			case *rsa.PrivateKey:
				sig, _ = rsa.SignPSS(rand.Reader, k, crypto.SHA256, sum[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
			case ed25519.PrivateKey:
				sig = ed25519.Sign(k, msg)
			}
			if !VerifyLocal(pub, x.p, msg, false, sig) || VerifyLocal(pub, x.p, []byte("wrong"), false, sig) {
				t.Fatal("signature validity not enforced")
			}
			if x.kind != "ed25519" && !VerifyLocal(pub, x.p, sum[:], true, sig) {
				t.Fatal("digest semantic mismatch")
			}
			if VerifyLocal(pub, x.p, []byte{1}, true, sig) {
				t.Fatal("bad digest accepted")
			}
		}
	}
	for _, s := range []string{"", "garbage", strings.Repeat("A", 17000), "AA==", "-----BEGIN PUBLIC KEY-----\nAA==\n-----END PUBLIC KEY-----"} {
		if _, _, _, e := Public("ed25519", s); e == nil {
			t.Fatal("invalid public key accepted")
		}
	}
	der, _ := x509.MarshalPKIXPublicKey(ec.Public())
	text := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	for _, kind := range []string{"rsa-2048", "ed25519", "hmac", "unknown"} {
		if _, _, _, e := Public(kind, text); e == nil {
			t.Fatal("key type mismatch accepted")
		}
	}
	for _, p := range []transit.Profile{transit.ECDSAP256SHA256ASN1, transit.RSAPSSSHA256, transit.Ed25519Message, "unknown"} {
		if SignatureShape(p, []byte{1}) {
			t.Fatal("bad signature shape accepted")
		}
		WireProfile(p, false)
	}
	if ProfileValid("unknown", false) || ProfileValid(transit.Ed25519Message, true) || Compatible("hmac", transit.RSAPSSSHA256) {
		t.Fatal("unsupported profile accepted")
	}
}
func FuzzEncoding(f *testing.F) {
	for _, s := range []string{"vault:v1:AA==", "vault:v01:AB==", "", "vault:v2:Zm9v"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			return
		}
		v, b, e := Unwrap(s, 1<<20)
		if e == nil {
			if v <= 0 || len(b) == 0 {
				t.Fatal("invalid successful envelope")
			}
			for _, p := range []transit.Profile{transit.ECDSAP256SHA256ASN1, transit.RSAPSSSHA256, transit.Ed25519Message} {
				SignatureShape(p, b)
			}
		}
	})
}
