package signer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIndependentSigner(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	public, e := x509.MarshalPKIXPublicKey(key.Public())
	if e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch r.URL.Path {
		case "/v1/transit/keys/platform":
			data = map[string]any{"name": "platform", "type": "ecdsa-p256", "latest_version": 1, "min_encryption_version": 0, "min_decryption_version": 1, "supports_signing": true, "supports_encryption": false, "exportable": false, "deletion_allowed": false, "derived": false, "keys": map[string]any{"1": map[string]any{"public_key": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})), "creation_time": "2026-01-01T00:00:00Z"}}}
		case "/v1/transit/sign/platform":
			var body struct {
				Input   string `json:"input"`
				Version int    `json:"key_version"`
			}
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Error("request JSON")
				w.WriteHeader(400)
				return
			}
			msg, e := base64.StdEncoding.DecodeString(body.Input)
			if e != nil || body.Version != 1 {
				t.Error("signing contract")
				w.WriteHeader(400)
				return
			}
			sum := sha256.Sum256(msg)
			sig, e := ecdsa.SignASN1(rand.Reader, key, sum[:])
			if e != nil {
				t.Error("fixture signing")
				w.WriteHeader(500)
				return
			}
			data = map[string]any{"signature": "vault:v1:" + base64.StdEncoding.EncodeToString(sig), "key_version": 1}
		default:
			t.Error("unexpected request")
			w.WriteHeader(404)
			return
		}
		if e := json.NewEncoder(w).Encode(map[string]any{"data": data}); e != nil {
			t.Error("response encoding")
		}
	}))
	defer srv.Close()
	token := sensitive.NewBytes([]byte("signer-fixture-token"))
	defer token.Zero()
	provider, e := auth.NewStaticToken(token)
	if e != nil {
		t.Fatal(e)
	}
	c, e := bao.New(bao.Config{Address: srv.URL, ClusterAlias: "consumer", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceRoot}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}, TLS: bao.TLSConfig{CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})}})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e = c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer func() {
		x, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if e := c.Close(x); e != nil {
			t.Error(e)
		}
	}()
	signer, e := New(c, "transit", "platform", 1)
	if e != nil {
		t.Fatal(e)
	}
	msg := sensitive.NewBytes([]byte("non-secret fixture"))
	defer msg.Zero()
	result, e := signer.Sign(ctx, msg)
	if e != nil {
		t.Fatal(e)
	}
	if result.Key.Version != 1 {
		t.Fatal("signature version")
	}
}
