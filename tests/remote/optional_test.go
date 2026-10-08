package remotecheck

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sendData(w http.ResponseWriter, data any) {
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func TestPermissionNegativeRequiresValidRestrictedIdentity(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "invalid"}[invalid], func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", "")
			h.c.negativeToken = "restricted-synthetic"
			h.c.negativeMount = "kv"
			h.c.negativePath = "forbidden/check"
			s.extra = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Header.Get("X-Vault-Token") != "restricted-synthetic" {
					return false
				}
				if r.URL.Path == "/v1/auth/token/lookup-self" && !invalid {
					sendData(w, map[string]any{"ttl": 10, "num_uses": 2})
					return true
				}
				w.WriteHeader(403)
				return true
			}
			r := h.run()
			if invalid && r.Status == "PASS" || !invalid && r.Status != "PASS" {
				t.Fatalf("permission cases %+v", r.Cases)
			}
		})
	}
}

func TestTransitOnlyUsesExistingKeyAndRejectsTampering(t *testing.T) {
	for _, kind := range []string{"aes256-gcm96", "ed25519"} {
		t.Run(kind, func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", "")
			h.c.transitMount = "transit"
			h.c.transitKey = "test-key"
			h.c.transitType = kind
			public, key, _ := ed25519.GenerateKey(rand.Reader)
			spki, _ := x509.MarshalPKIXPublicKey(public)
			publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki}))
			block, _ := aes.NewCipher(make([]byte, 32))
			aead, _ := cipher.NewGCM(block)
			s.extra = func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.HasPrefix(r.URL.Path, "/v1/transit/") {
					return false
				}
				if r.Method == "GET" && r.URL.Path == "/v1/transit/keys/test-key" {
					keys := map[string]any{"1": 1700000000}
					if kind == "ed25519" {
						keys["1"] = map[string]any{"public_key": publicPEM, "creation_time": "2026-01-01T00:00:00Z"}
					}
					sendData(w, map[string]any{"name": "test-key", "type": kind, "keys": keys, "latest_version": 1, "min_encryption_version": 0, "min_decryption_version": 1, "exportable": false, "deletion_allowed": false, "supports_signing": kind == "ed25519", "supports_encryption": kind == "aes256-gcm96", "derived": false, "convergent_encryption": false})
					return true
				}
				var input map[string]any
				json.NewDecoder(r.Body).Decode(&input)
				switch r.URL.Path {
				case "/v1/transit/encrypt/test-key":
					raw, _ := base64.StdEncoding.DecodeString(input["plaintext"].(string))
					nonce := make([]byte, aead.NonceSize())
					rand.Read(nonce)
					wrapped := "vault:v1:" + base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, raw, nil))
					sendData(w, map[string]any{"ciphertext": wrapped, "key_version": 1})
				case "/v1/transit/decrypt/test-key":
					raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(input["ciphertext"].(string), "vault:v1:"))
					if len(raw) < aead.NonceSize() {
						w.WriteHeader(400)
						return true
					}
					plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
					if err != nil {
						w.WriteHeader(400)
						sendData(w, map[string]any{})
						return true
					}
					sendData(w, map[string]any{"plaintext": base64.StdEncoding.EncodeToString(plain)})
				case "/v1/transit/sign/test-key":
					raw, _ := base64.StdEncoding.DecodeString(input["input"].(string))
					sendData(w, map[string]any{"signature": "vault:v1:" + base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)), "key_version": 1})
				case "/v1/transit/verify/test-key":
					raw, _ := base64.StdEncoding.DecodeString(input["input"].(string))
					signature, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(input["signature"].(string), "vault:v1:"))
					sendData(w, map[string]any{"valid": ed25519.Verify(public, raw, signature)})
				default:
					t.Error("unexpected Transit mutation")
					w.WriteHeader(403)
				}
				return true
			}
			r := h.run()
			if r.Status != "PASS" || r.Requests != s.requests {
				t.Fatalf("Transit cases: %+v", r.Cases)
			}
		})
	}
}

func TestPKISignsLocalCSRValidatesTrustAndRevokesOwnSerial(t *testing.T) {
	for _, wrongDNS := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "wrong-san"}[wrongDNS], func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", "")
			h.c.pkiMount = "pki"
			h.c.pkiRole = "test-role"
			h.c.pkiDNS = "sdk.example.test"
			h.c.pkiRevoke = true
			key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			now := time.Now()
			root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "local test root"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
			der, _ := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
			rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
			h.c.pkiRoot = filepath.Join(t.TempDir(), "trusted.pem")
			os.WriteFile(h.c.pkiRoot, rootPEM, 0600)
			revoked := false
			s.extra = func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.HasPrefix(r.URL.Path, "/v1/pki/") {
					return false
				}
				if r.URL.Path == "/v1/pki/sign/test-role" {
					var req struct {
						CSR string `json:"csr"`
					}
					json.NewDecoder(r.Body).Decode(&req)
					block, _ := pem.Decode([]byte(req.CSR))
					csr, err := x509.ParseCertificateRequest(block.Bytes)
					if err != nil || csr.CheckSignature() != nil {
						t.Error("invalid local CSR")
						w.WriteHeader(400)
						return true
					}
					dns := csr.DNSNames
					if wrongDNS {
						dns = []string{"wrong.example.test"}
					}
					cert := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: csr.Subject, DNSNames: dns, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(5 * time.Minute), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
					issued, _ := x509.CreateCertificate(rand.Reader, cert, root, csr.PublicKey, key)
					sendData(w, map[string]any{"certificate": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issued})), "issuing_ca": string(rootPEM), "ca_chain": []string{string(rootPEM)}, "serial_number": "02", "expiration": cert.NotAfter.Unix()})
					return true
				}
				if r.URL.Path == "/v1/pki/revoke" {
					var req map[string]string
					json.NewDecoder(r.Body).Decode(&req)
					if req["serial_number"] != "02" {
						t.Error("foreign revocation")
					}
					revoked = true
					sendData(w, map[string]any{"revocation_time": time.Now().Unix(), "revocation_time_rfc3339": time.Now().UTC().Format(time.RFC3339)})
					return true
				}
				t.Error("unexpected PKI operation")
				w.WriteHeader(403)
				return true
			}
			r := h.run()
			if !revoked || r.Requests != s.requests || (!wrongDNS && r.Status != "PASS") || (wrongDNS && r.Status == "PASS") {
				t.Fatalf("PKI cases: %+v", r.Cases)
			}
		})
	}
}
