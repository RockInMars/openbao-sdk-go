package bao

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func TestPublicKeyDecodeAttemptsNotHTTPStatus(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		jsonData(w, map[string]any{"name": "key", "type": "ecdsa-p256", "latest_version": 1, "min_encryption_version": 0, "min_decryption_version": 1, "supports_signing": true, "supports_encryption": false, "exportable": false, "deletion_allowed": false, "derived": false, "keys": map[string]any{"1": map[string]any{"public_key": "broken"}}})
	}))
	tr, e := c.Transit("transit")
	if e != nil {
		t.Fatal(e)
	}
	_, e = tr.ReadPublicKey(context.Background(), "key", 1)
	var be *baoerr.Error
	if !errors.As(e, &be) || be.Attempts != 1 || be.HTTPStatus != 200 {
		t.Fatal("public key decode confused HTTP status with attempts")
	}
}
func TestTransitMetadataRejectsMalformedFields(t *testing.T) {
	cases := []struct {
		name, field string
		value       any
		omit        bool
	}{
		{"name", "name", "different", false}, {"type", "type", nil, false}, {"latest", "latest_version", 0, false},
		{"minimum encryption", "min_encryption_version", 3, false}, {"minimum decryption", "min_decryption_version", -1, false},
		{"boolean", "exportable", "false", false}, {"derived", "derived", nil, false}, {"convergent", "convergent_encryption", 1, false},
		{"keys omitted", "keys", nil, true}, {"keys empty", "keys", map[string]any{}, false}, {"version padded", "keys", map[string]any{"01": 1}, false},
		{"version future", "keys", map[string]any{"3": 1}, false}, {"version zero", "keys", map[string]any{"0": 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := keyMeta("cipher", "aes256-gcm96", map[string]any{"1": 1, "2": 2}, false)
			if tc.omit {
				delete(m, tc.field)
			} else {
				m[tc.field] = tc.value
			}
			var requests atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { requests.Add(1); jsonData(w, m) }))
			tr, _ := c.Transit("transit")
			if got, err := tr.ReadKeyMetadata(context.Background(), "cipher"); got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || requests.Load() != 1 {
				t.Fatalf("bad metadata accepted/retried: %v", err)
			}
		})
	}
}

func TestTransitHMACMetadataMayOmitVersionHistory(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  any
		omit  bool
		valid bool
	}{
		{name: "omitted", omit: true, valid: true},
		{name: "empty", keys: map[string]any{}, valid: true},
		{name: "null", keys: nil},
		{name: "malformed", keys: "1"},
		{name: "version padded", keys: map[string]any{"01": 1}},
		{name: "version future", keys: map[string]any{"3": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := keyMeta("mac", "hmac", map[string]any{"1": 1, "2": 2}, false)
			if tc.omit {
				delete(m, "keys")
			} else {
				m["keys"] = tc.keys
			}
			var requests atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				jsonData(w, m)
			}))
			tr, _ := c.Transit("transit")
			got, err := tr.ReadKeyMetadata(context.Background(), "mac")
			if tc.valid {
				if err != nil || got == nil || got.Type != "hmac" || got.LatestVersion != 2 || len(got.VersionNumbers) != 0 || requests.Load() != 1 {
					t.Fatalf("valid HMAC metadata rejected or versions invented: %v", err)
				}
			} else if got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || requests.Load() != 1 {
				t.Fatalf("malformed HMAC metadata accepted or retried: %v", err)
			}
		})
	}
}

func TestTransitHMACWithoutHistoryUsesOnlyLatestVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		omit bool
	}{{"omitted", true}, {"empty", false}} {
		t.Run(tc.name, func(t *testing.T) {
			m := keyMeta("mac", "hmac", map[string]any{"1": 1, "2": 2}, false)
			if tc.omit {
				delete(m, "keys")
			} else {
				m["keys"] = map[string]any{}
			}
			wrapped := "vault:v2:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
			var reads, writes atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					reads.Add(1)
					jsonData(w, m)
					return
				}
				writes.Add(1)
				switch r.URL.Path {
				case "/v1/transit/hmac/mac/sha2-256":
					jsonData(w, map[string]any{"hmac": wrapped})
				case "/v1/transit/verify/mac/sha2-256":
					jsonData(w, map[string]any{"valid": true})
				default:
					t.Errorf("unexpected write path: %s", r.URL.Path)
				}
			}))
			tr, _ := c.Transit("transit")
			message := sensitive.NewBytes([]byte("fixture"))
			defer message.Zero()
			mac, err := tr.HMAC(context.Background(), transit.HMACRequest{KeyName: "mac", KeyVersion: 2, Message: message})
			if err != nil || mac == nil || mac.Wrapped != wrapped {
				t.Fatalf("latest HMAC failed: %v", err)
			}
			verified, err := tr.HMACVerify(context.Background(), transit.HMACVerifyRequest{KeyName: "mac", ExpectedVersion: 2, Message: message, WrappedHMAC: wrapped})
			if err != nil || verified == nil || !verified.Valid || writes.Load() != 2 {
				t.Fatalf("latest HMAC verification failed: %v", err)
			}
			if got, err := tr.HMAC(context.Background(), transit.HMACRequest{KeyName: "mac", KeyVersion: 1, Message: message}); got != nil || !baoerr.IsCode(err, baoerr.CodeVersionUnavailable) || writes.Load() != 2 || reads.Load() != 3 {
				t.Fatalf("unlisted historical HMAC version was sent: %v", err)
			}
		})
	}
}

func TestTransitPublicKeyRequiresRequestedVersionAndMaterial(t *testing.T) {
	for _, entry := range []any{1, map[string]any{}, map[string]any{"public_key": "invalid"}} {
		t.Run(stringifyValue(entry), func(t *testing.T) {
			m := keyMeta("key", "ed25519", map[string]any{"1": entry}, false)
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { jsonData(w, m) }))
			tr, _ := c.Transit("transit")
			if got, err := tr.ReadPublicKey(context.Background(), "key", 1); got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) {
				t.Fatalf("invalid public material accepted: %v", err)
			}
			if got, err := tr.ReadPublicKey(context.Background(), "key", 2); got != nil || !baoerr.IsCode(err, baoerr.CodeVersionUnavailable) {
				t.Fatalf("missing version silently replaced: %v", err)
			}
		})
	}
}

func TestTransitCipherRejectsMalformedReplies(t *testing.T) {
	wrapped := "vault:v1:" + base64.StdEncoding.EncodeToString(make([]byte, 28))
	for _, data := range []map[string]any{{}, {"ciphertext": 1}, {"ciphertext": "broken"}, {"ciphertext": wrapped, "key_version": 2}, {"ciphertext": wrapped, "key_version": "1"}} {
		t.Run(stringifyValue(data), func(t *testing.T) {
			var writes atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					jsonData(w, keyMeta("cipher", "aes256-gcm96", map[string]any{"1": 1, "2": 2}, false))
					return
				}
				writes.Add(1)
				jsonData(w, data)
			}))
			tr, _ := c.Transit("transit")
			got, err := tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1, Plaintext: sensitive.NewBytes([]byte("fixture"))})
			if got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || writes.Load() != 1 {
				t.Fatalf("cipher reply accepted or replayed: %v", err)
			}
		})
	}
	for _, value := range []any{nil, "%%%", "AB=="} {
		t.Run("plaintext-"+stringifyValue(value), func(t *testing.T) {
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					jsonData(w, keyMeta("cipher", "aes256-gcm96", map[string]any{"1": 1, "2": 2}, false))
					return
				}
				jsonData(w, map[string]any{"plaintext": value})
			}))
			tr, _ := c.Transit("transit")
			got, err := tr.Decrypt(context.Background(), transit.DecryptRequest{KeyName: "cipher", Ciphertext: transit.Ciphertext{Wrapped: wrapped, Version: 1}})
			if got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) {
				t.Fatalf("invalid base64 plaintext accepted: %v", err)
			}
		})
	}
}

func TestTransitRetiredOrUnsupportedKeyNeverMutates(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
		code  string
	}{
		{"min_encryption_version", 2, baoerr.CodeVersionUnavailable},
		{"supports_encryption", false, baoerr.CodeInvalidArgument}, {"type", "unsupported", baoerr.CodeInvalidArgument},
		{"convergent_encryption", true, baoerr.CodeInvalidArgument}, {"derived", true, baoerr.CodeInvalidArgument},
	} {
		t.Run(tc.field, func(t *testing.T) {
			var writes atomic.Int32
			m := keyMeta("cipher", "aes256-gcm96", map[string]any{"1": 1, "2": 2}, false)
			m[tc.field] = tc.value
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes.Add(1)
				}
				jsonData(w, m)
			}))
			tr, _ := c.Transit("transit")
			got, err := tr.Encrypt(context.Background(), transit.EncryptRequest{KeyName: "cipher", KeyVersion: 1})
			if got != nil || !baoerr.IsCode(err, tc.code) || writes.Load() != 0 {
				t.Fatalf("preflight failed open: %v", err)
			}
		})
	}
}
