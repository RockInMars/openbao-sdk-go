// Package transitutil implements strict, bounded wire encodings. It does not
// create, export, or hold private keys.
package transitutil

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"git.example.com/infra/openbao-sdk-go/internal/pemutil"
	"math/big"
	"strconv"
	"strings"

	"git.example.com/infra/openbao-sdk-go/transit"
)

var errEncoding = errors.New("invalid transit encoding")

// Unwrap accepts one canonical vault:vN:standard-base64 envelope. The returned
// buffer is newly allocated; callers control its lifetime.
func Unwrap(s string, max int) (int, []byte, error) {
	if len(s) == 0 || len(s) > max || max <= 0 {
		return 0, nil, errEncoding
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 || parts[0] != "vault" || len(parts[1]) < 2 || parts[1][0] != 'v' {
		return 0, nil, errEncoding
	}
	n, e := strconv.Atoi(parts[1][1:])
	if e != nil || n <= 0 || strconv.Itoa(n) != parts[1][1:] {
		return 0, nil, errEncoding
	}
	b, e := base64.StdEncoding.Strict().DecodeString(parts[2])
	if e != nil || len(b) == 0 || base64.StdEncoding.EncodeToString(b) != parts[2] {
		clear(b)
		return 0, nil, errEncoding
	}
	return n, b, nil
}
func ProfileValid(p transit.Profile, digest bool) bool {
	switch p {
	case transit.ECDSAP256SHA256ASN1, transit.RSAPSSSHA256:
		return true
	case transit.Ed25519Message:
		return !digest
	}
	return false
}
func WireProfile(p transit.Profile, digest bool) map[string]any {
	b := map[string]any{"prehashed": digest}
	switch p {
	case transit.ECDSAP256SHA256ASN1:
		b["hash_algorithm"] = "sha2-256"
		b["marshaling_algorithm"] = "asn1"
	case transit.RSAPSSSHA256:
		b["hash_algorithm"] = "sha2-256"
		b["signature_algorithm"] = "pss"
		b["salt_length"] = "hash"
	case transit.Ed25519Message:
		b["hash_algorithm"] = "none"
	}
	return b
}
func SignatureShape(p transit.Profile, b []byte) bool {
	switch p {
	case transit.ECDSAP256SHA256ASN1:
		var pair struct{ R, S *big.Int }
		rest, e := asn1.Unmarshal(b, &pair)
		if e != nil || len(rest) != 0 || pair.R == nil || pair.S == nil {
			return false
		}
		n := elliptic.P256().Params().N
		return pair.R.Sign() > 0 && pair.S.Sign() > 0 && pair.R.Cmp(n) < 0 && pair.S.Cmp(n) < 0
	case transit.RSAPSSSHA256:
		return len(b) >= 256 && len(b) <= 1024
	case transit.Ed25519Message:
		return len(b) == ed25519.SignatureSize
	}
	return false
}

// Public parses the declared algorithm, not a guessed PEM representation.
func Public(kind, text string) (crypto.PublicKey, []byte, []byte, error) {
	if len(text) == 0 || len(text) > 16384 {
		return nil, nil, nil, errEncoding
	}
	var pub crypto.PublicKey
	if kind == "ed25519" && !strings.Contains(text, "-----BEGIN") {
		b, e := base64.StdEncoding.Strict().DecodeString(text)
		if e != nil || len(b) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(b) != text {
			return nil, nil, nil, errEncoding
		}
		pub = ed25519.PublicKey(b)
	} else {
		raw := bytes.TrimSpace([]byte(text))
		block, rest := pemutil.Decode(raw)
		if block == nil || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 || !bytes.HasPrefix(raw, []byte("-----BEGIN PUBLIC KEY-----")) {
			return nil, nil, nil, errEncoding
		}
		var e error
		pub, e = x509.ParsePKIXPublicKey(block.Bytes)
		if e != nil {
			return nil, nil, nil, errEncoding
		}
	}
	switch kind {
	case "ecdsa-p256":
		k, ok := pub.(*ecdsa.PublicKey)
		if !ok || k.Curve != elliptic.P256() || !k.Curve.IsOnCurve(k.X, k.Y) {
			return nil, nil, nil, errEncoding
		}
	case "rsa-2048", "rsa-3072", "rsa-4096":
		k, ok := pub.(*rsa.PublicKey)
		want, _ := strconv.Atoi(strings.TrimPrefix(kind, "rsa-"))
		if !ok || k.N == nil || k.N.BitLen() != want || k.E < 3 || k.E%2 == 0 {
			return nil, nil, nil, errEncoding
		}
	case "ed25519":
		if k, ok := pub.(ed25519.PublicKey); !ok || len(k) != 32 {
			return nil, nil, nil, errEncoding
		}
	default:
		return nil, nil, nil, errEncoding
	}
	der, e := x509.MarshalPKIXPublicKey(pub)
	if e != nil {
		return nil, nil, nil, errEncoding
	}
	return pub, der, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
func Compatible(kind string, p transit.Profile) bool {
	switch p {
	case transit.ECDSAP256SHA256ASN1:
		return kind == "ecdsa-p256"
	case transit.RSAPSSSHA256:
		return kind == "rsa-2048" || kind == "rsa-3072" || kind == "rsa-4096"
	case transit.Ed25519Message:
		return kind == "ed25519"
	}
	return false
}
func VerifyLocal(pub crypto.PublicKey, p transit.Profile, input []byte, prehashed bool, sig []byte) bool {
	if !SignatureShape(p, sig) || !ProfileValid(p, prehashed) {
		return false
	}
	if p == transit.Ed25519Message {
		k, ok := pub.(ed25519.PublicKey)
		return ok && ed25519.Verify(k, input, sig)
	}
	d := input
	if !prehashed {
		h := sha256.Sum256(input)
		d = h[:]
	} else if len(d) != 32 {
		return false
	}
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		return p == transit.ECDSAP256SHA256ASN1 && ecdsa.VerifyASN1(k, d, sig)
	case *rsa.PublicKey:
		return p == transit.RSAPSSSHA256 && rsa.VerifyPSS(k, crypto.SHA256, d, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}) == nil
	}
	return false
}
