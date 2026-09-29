package bao

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"git.example.com/infra/openbao-sdk-go/transit"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type signFixture struct {
	profile transit.Profile
	typ     string
	key     crypto.Signer
}

func signFixtures(t *testing.T) []signFixture {
	t.Helper()
	ec, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	rs, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	_, ed, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return []signFixture{{transit.ECDSAP256SHA256ASN1, "ecdsa-p256", ec}, {transit.RSAPSSSHA256, "rsa-2048", rs}, {transit.Ed25519Message, "ed25519", ed}}
}
func publicText(t *testing.T, k crypto.PublicKey, kind string) string {
	t.Helper()
	if kind == "ed25519" {
		return base64.StdEncoding.EncodeToString(k.(ed25519.PublicKey))
	}
	b, e := x509.MarshalPKIXPublicKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}))
}
func keyMeta(name, kind string, keys map[string]any, derived bool) map[string]any {
	return map[string]any{"name": name, "type": kind, "keys": keys, "latest_version": 2, "min_encryption_version": 0, "min_decryption_version": 1, "exportable": false, "deletion_allowed": false, "supports_signing": !strings.HasPrefix(kind, "aes") && kind != "hmac", "supports_encryption": strings.HasPrefix(kind, "aes"), "derived": derived, "convergent_encryption": false}
}
func jsonData(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": v}) }
func signLocal(t *testing.T, f signFixture, input []byte, prehashed bool) []byte {
	t.Helper()
	if f.profile == transit.Ed25519Message {
		return ed25519.Sign(f.key.(ed25519.PrivateKey), input)
	}
	var digest []byte
	if prehashed {
		digest = input
	} else {
		h := sha256.Sum256(input)
		digest = h[:]
	}
	if f.profile == transit.ECDSAP256SHA256ASN1 {
		b, e := ecdsa.SignASN1(rand.Reader, f.key.(*ecdsa.PrivateKey), digest)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	b, e := rsa.SignPSS(rand.Reader, f.key.(*rsa.PrivateKey), crypto.SHA256, digest, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestTransitSignProfiles(t *testing.T) {
	for _, f := range signFixtures(t) {
		t.Run(string(f.profile), func(t *testing.T) {
			msg := []byte("message bytes, not pre-encoded")
			pub := publicText(t, f.key.Public(), f.typ)
			var writes atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if r.URL.Path != "/v1/transit/keys/signer" {
						t.Error("invented version endpoint")
					}
					jsonData(w, keyMeta("signer", f.typ, map[string]any{"1": map[string]any{"public_key": pub}, "2": map[string]any{"public_key": pub}}, false))
					return
				}
				writes.Add(1)
				var b map[string]any
				if json.NewDecoder(r.Body).Decode(&b) != nil {
					t.Error("bad sign JSON")
				}
				input, e := base64.StdEncoding.DecodeString(b["input"].(string))
				if e != nil {
					t.Error("bad input base64")
				}
				pre, ok := b["prehashed"].(bool)
				if !ok {
					t.Error("prehashed implicit")
				}
				if b["key_version"] != float64(2) {
					t.Error("wrong sign version")
				}
				if f.profile == transit.Ed25519Message {
					if pre {
						t.Error("ed prehashed")
					}
				} else {
					if b["hash_algorithm"] != "sha2-256" {
						t.Error("hash profile")
					}
					if f.profile == transit.ECDSAP256SHA256ASN1 && b["marshaling_algorithm"] != "asn1" {
						t.Error("DER profile")
					}
					if f.profile == transit.RSAPSSSHA256 && (b["signature_algorithm"] != "pss" || b["salt_length"] != "hash") {
						t.Error("PSS profile")
					}
				}
				if !pre && !bytes.Equal(input, msg) {
					t.Error("input encoded twice")
				}
				sig := signLocal(t, f, input, pre)
				jsonData(w, map[string]any{"signature": "vault:v2:" + base64.StdEncoding.EncodeToString(sig), "key_version": 2})
			}))
			tr, e := c.Transit("transit")
			if e != nil {
				t.Fatal(e)
			}
			r, e := tr.Sign(context.Background(), transit.SignRequest{KeyName: "signer", KeyVersion: 2, Profile: f.profile, Message: sensitive.NewBytes(msg)})
			if e != nil || r.Signature.Version != 2 || r.Key.Version != 2 {
				t.Fatalf("sign result: %v", e)
			}
			h := sha256.Sum256(msg)
			_, e = tr.SignDigest(context.Background(), transit.SignDigestRequest{KeyName: "signer", KeyVersion: 2, Profile: f.profile, Digest: sensitive.NewBytes(h[:])})
			if f.profile == transit.Ed25519Message {
				if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || writes.Load() != 1 {
					t.Fatal("ed digest sent")
				}
			} else if e != nil || writes.Load() != 2 {
				t.Fatal("digest sign failed")
			}
		})
	}
}
func TestTransitVersionPin(t *testing.T) {
	f := signFixtures(t)[0]
	pub := publicText(t, f.key.Public(), f.typ)
	var count atomic.Int32
	sig := base64.StdEncoding.EncodeToString(signLocal(t, f, []byte("fixture"), false))
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.Method == "GET" {
			jsonData(w, keyMeta("signer", f.typ, map[string]any{"2": map[string]any{"public_key": pub}}, false))
			return
		}
		jsonData(w, map[string]any{"signature": "vault:v3:" + sig})
	}))
	tr, _ := c.Transit("transit")
	if _, e := tr.Sign(context.Background(), transit.SignRequest{KeyName: "signer", Profile: f.profile}); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || count.Load() != 0 {
		t.Fatal("zero version sent")
	}
	r, e := tr.Sign(context.Background(), transit.SignRequest{KeyName: "signer", KeyVersion: 2, Profile: f.profile, Message: sensitive.NewBytes([]byte("fixture"))})
	if r != nil || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("wrong wrapped version accepted")
	}
	before := count.Load()
	_, e = tr.Verify(context.Background(), transit.VerifyRequest{KeyName: "signer", ExpectedVersion: 2, Profile: f.profile, Signature: transit.Signature{Wrapped: "vault:v1:" + sig, Version: 1, Profile: f.profile}})
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || count.Load() != before {
		t.Fatal("verify version silently switched")
	}
}
func TestTransitPublicKeyFormats(t *testing.T) {
	for _, f := range signFixtures(t) {
		pub := publicText(t, f.key.Public(), f.typ)
		c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
			jsonData(w, keyMeta("signer", f.typ, map[string]any{"1": map[string]any{"public_key": pub}, "2": map[string]any{"public_key": "not historical key"}}, false))
		}))
		tr, _ := c.Transit("transit")
		r, e := tr.ReadPublicKey(context.Background(), "signer", 1)
		if e != nil || r.Key.Version != 1 {
			t.Fatalf("key decode: %v", e)
		}
		k, e := x509.ParsePKIXPublicKey(r.SPKIDER)
		if e != nil {
			t.Fatal("SPKI not normalized")
		}
		want, _ := x509.MarshalPKIXPublicKey(f.key.Public())
		got, _ := x509.MarshalPKIXPublicKey(k)
		if !bytes.Equal(want, got) {
			t.Fatal("wrong historical public key")
		}
	}
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		jsonData(w, keyMeta("secret", "aes256-gcm96", map[string]any{"1": 1700000000, "2": 1700000001}, false))
	}))
	tr, _ := c.Transit("transit")
	if _, e := tr.ReadPublicKey(context.Background(), "secret", 1); e == nil {
		t.Fatal("timestamp treated as public key")
	}
}
func TestTransitVerifyFalse(t *testing.T) {
	f := signFixtures(t)[2]
	pub := publicText(t, f.key.Public(), f.typ)
	var fail atomic.Bool
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonData(w, keyMeta("signer", f.typ, map[string]any{"1": map[string]any{"public_key": pub}, "2": map[string]any{"public_key": pub}}, false))
			return
		}
		if fail.Load() {
			w.WriteHeader(503)
			w.Write([]byte(`{"errors":["temporary"]}`))
			return
		}
		jsonData(w, map[string]any{"valid": false})
	}))
	tr, _ := c.Transit("transit")
	sig := "vault:v2:" + base64.StdEncoding.EncodeToString(signLocal(t, f, []byte("fixture"), false))
	req := transit.VerifyRequest{KeyName: "signer", ExpectedVersion: 2, Profile: f.profile, Message: sensitive.NewBytes([]byte("tampered")), Signature: transit.Signature{Wrapped: sig, Version: 2, Profile: f.profile}}
	r, e := tr.Verify(context.Background(), req)
	if e != nil || r.Valid {
		t.Fatal("false treated as failure")
	}
	fail.Store(true)
	if r, e = tr.Verify(context.Background(), req); e == nil || r != nil {
		t.Fatal("network failure swallowed as false")
	}
}
func fixtureWrapped(v int, b []byte) string {
	return fmt.Sprintf("vault:v%d:%s", v, base64.StdEncoding.EncodeToString(b))
}
