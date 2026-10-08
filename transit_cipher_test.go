package bao

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/transitutil"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

// This is a protocol fixture using standard-library crypto, not a fake report of
// a real OpenBao integration. It detects version and context wire mistakes.
func cipherFixture(t *testing.T, derived bool) (*TransitClient, *atomic.Int32) {
	t.Helper()
	master := make([]byte, 32)
	if _, e := rand.Read(master); e != nil {
		t.Fatal(e)
	}
	var writes atomic.Int32
	aead := func(v int, ctx string) cipher.AEAD {
		mac := hmac.New(sha256.New, master)
		mac.Write([]byte{byte(v)})
		mac.Write([]byte(ctx))
		block, e := aes.NewCipher(mac.Sum(nil))
		if e != nil {
			panic(e)
		}
		g, e := cipher.NewGCM(block)
		if e != nil {
			panic(e)
		}
		return g
	}
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonData(w, keyMeta("cipher", "aes256-gcm96", map[string]any{"1": 1700000000, "2": 1700000001}, derived))
			return
		}
		writes.Add(1)
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			t.Error("invalid cipher payload")
			w.WriteHeader(400)
			return
		}
		ctx, _ := b["context"].(string)
		if derived && ctx == "" {
			t.Error("derived context missing")
		}
		if !derived && ctx != "" {
			t.Error("non-derived context sent")
		}
		fail := func() { w.WriteHeader(400); w.Write([]byte(`{"errors":["ciphertext invalid"]}`)) }
		seal := func(v int, p []byte) string {
			g := aead(v, ctx)
			nonce := make([]byte, g.NonceSize())
			rand.Read(nonce)
			sealed := g.Seal(nonce, nonce, p, nil)
			return fixtureWrapped(v, sealed)
		}
		if strings.Contains(r.URL.Path, "/encrypt/") {
			v, ok := b["key_version"].(float64)
			if !ok {
				t.Error("encrypt missing explicit version")
			}
			raw, ok := b["plaintext"].(string)
			if !ok {
				t.Error("missing plaintext")
			}
			p, e := base64.StdEncoding.DecodeString(raw)
			if e != nil {
				fail()
				return
			}
			jsonData(w, map[string]any{"ciphertext": seal(int(v), p), "key_version": int(v)})
			return
		}
		wrapped, _ := b["ciphertext"].(string)
		v, blob, e := transitutil.Unwrap(wrapped, 1<<20)
		if e != nil {
			fail()
			return
		}
		g := aead(v, ctx)
		if len(blob) < g.NonceSize() {
			fail()
			return
		}
		p, e := g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], nil)
		if e != nil {
			fail()
			return
		}
		if strings.Contains(r.URL.Path, "/decrypt/") {
			if _, exists := b["key_version"]; exists {
				t.Error("invented decrypt key_version")
			}
			jsonData(w, map[string]any{"plaintext": base64.StdEncoding.EncodeToString(p)})
			return
		}
		if strings.Contains(r.URL.Path, "/rewrap/") {
			v, ok := b["key_version"].(float64)
			if !ok {
				t.Error("rewrap target implicit")
			}
			jsonData(w, map[string]any{"ciphertext": seal(int(v), p), "key_version": int(v)})
			return
		}
		t.Error("unexpected cipher write")
		fail()
	}))
	tr, _ := c.Transit("transit")
	return tr, &writes
}
func TestTransitCipherVersion(t *testing.T) {
	tr, n := cipherFixture(t, false)
	plain := sensitive.NewBytes([]byte("cipher fixture payload"))
	r, e := tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1, Plaintext: plain})
	if e != nil || r.Ciphertext.Version != 1 {
		t.Fatalf("encrypt: %v", e)
	}
	d, e := tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: r.Ciphertext})
	if e != nil || !bytes.Equal(d.Plaintext.RevealCopy(), plain.RevealCopy()) {
		t.Fatalf("decrypt: %v", e)
	}
	d.Plaintext.Zero()
	rr, e := tr.Rewrap(context.Background(), transit.RewrapRequest{KeyName: "cipher", TargetVersion: 2, Ciphertext: r.Ciphertext})
	if e != nil || rr.Ciphertext.Version != 2 {
		t.Fatalf("rewrap: %v", e)
	}
	d, e = tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: rr.Ciphertext})
	if e != nil || !bytes.Equal(d.Plaintext.RevealCopy(), plain.RevealCopy()) {
		t.Fatalf("rewrapped plaintext: %v", e)
	}
	d.Plaintext.Zero()
	before := n.Load()
	_, e = tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher"})
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || n.Load() != before {
		t.Fatal("zero version accepted")
	}
	bad := rr.Ciphertext
	bad.Version = 1
	_, e = tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: bad})
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || n.Load() != before {
		t.Fatal("cipher version ambiguity")
	}
}
func TestTransitCipherContext(t *testing.T) {
	tr, n := cipherFixture(t, true)
	ctx := sensitive.NewBytes([]byte("fixture-context-A"))
	p := sensitive.NewBytes([]byte("derived fixture"))
	r, e := tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1, Context: ctx, Plaintext: p})
	if e != nil {
		t.Fatal(e)
	}
	_, e = tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: r.Ciphertext, Context: sensitive.NewBytes([]byte("fixture-context-B"))})
	if e == nil {
		t.Fatal("wrong context decrypted")
	}
	d, e := tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: r.Ciphertext, Context: ctx})
	if e != nil || !bytes.Equal(d.Plaintext.RevealCopy(), p.RevealCopy()) {
		t.Fatalf("correct context failed: %v", e)
	}
	d.Plaintext.Zero()
	before := n.Load()
	_, e = tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1, Plaintext: p})
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || n.Load() != before {
		t.Fatal("missing derived context sent")
	}
	non, m := cipherFixture(t, false)
	_, e = non.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1, Context: ctx, Plaintext: p})
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || m.Load() != 0 {
		t.Fatal("context sent to non-derived key")
	}
}
func TestTransitNoKeyCreation(t *testing.T) {
	var posts atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			posts.Add(1)
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"errors":[]}`))
	}))
	tr, _ := c.Transit("transit")
	_, e := tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "unknown", KeyVersion: 1, Plaintext: sensitive.NewBytes([]byte("x"))})
	if e == nil || posts.Load() != 0 {
		t.Fatal("unknown key attempted creation/write")
	}
}
func TestTransitHMAC(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	var fail atomic.Bool
	var writes atomic.Int32
	sum := func(msg []byte) []byte { m := hmac.New(sha256.New, key); m.Write(msg); return m.Sum(nil) }
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonData(w, keyMeta("mac", "hmac", map[string]any{"1": 1700000000, "2": 1700000001}, false))
			return
		}
		writes.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			w.Write([]byte(`{"errors":["unavailable"]}`))
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/sha2-256") {
			t.Error("hash not fixed")
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		input, _ := base64.StdEncoding.DecodeString(b["input"].(string))
		if strings.Contains(r.URL.Path, "/hmac/") {
			if b["key_version"] != float64(2) {
				t.Error("HMAC key version implicit")
			}
			jsonData(w, map[string]any{"hmac": fixtureWrapped(2, sum(input))})
			return
		}
		if _, ok := b["signature"]; ok {
			t.Error("HMAC sent as signature")
		}
		_, mac, e := transitutil.Unwrap(b["hmac"].(string), 1<<20)
		jsonData(w, map[string]any{"valid": e == nil && hmac.Equal(mac, sum(input))})
	}))
	tr, _ := c.Transit("transit")
	msg := sensitive.NewBytes([]byte("MAC fixture message"))
	r, e := tr.HMAC(context.Background(), transit.HMACRequest{KeyName: "mac", KeyVersion: 2, Message: msg})
	if e != nil || r.Version != 2 {
		t.Fatalf("HMAC: %v", e)
	}
	verify := transit.HMACVerifyRequest{KeyName: "mac", ExpectedVersion: 2, Message: msg, WrappedHMAC: r.Wrapped}
	v, e := tr.HMACVerify(context.Background(), verify)
	if e != nil || !v.Valid {
		t.Fatalf("valid HMAC: %v", e)
	}
	verify.Message = sensitive.NewBytes([]byte("modified"))
	v, e = tr.HMACVerify(context.Background(), verify)
	if e != nil || v.Valid {
		t.Fatal("tampered MAC not false")
	}
	before := writes.Load()
	fail.Store(true)
	v, e = tr.HMACVerify(context.Background(), verify)
	if e == nil || v != nil || writes.Load() != before+1 {
		t.Fatal("HMAC error swallowed/replayed")
	}
}
