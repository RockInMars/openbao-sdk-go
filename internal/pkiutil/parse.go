// Package pkiutil parses bounded certificate material without network access.
package pkiutil

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"github.com/RockInMars/openbao-sdk-go/internal/pemutil"
	"math/big"
	"strings"
)

var ErrMaterial = errors.New("invalid certificate material")

const MaxMaterialBytes = 4 << 20

func Certificates(raw []byte) ([]*x509.Certificate, [][]byte, error) {
	if len(raw) == 0 || len(raw) > MaxMaterialBytes {
		return nil, nil, ErrMaterial
	}
	rest := bytes.TrimSpace(raw)
	var certs []*x509.Certificate
	var blocks [][]byte
	for len(rest) > 0 {
		if len(certs) >= 64 || !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, nil, ErrMaterial
		}
		b, next := pemutil.Decode(rest)
		if b == nil || b.Type != "CERTIFICATE" || len(b.Headers) != 0 {
			return nil, nil, ErrMaterial
		}
		c, e := x509.ParseCertificate(b.Bytes)
		if e != nil || c.SerialNumber == nil || c.SerialNumber.Sign() <= 0 || !c.NotAfter.After(c.NotBefore) {
			return nil, nil, ErrMaterial
		}
		certs = append(certs, c)
		blocks = append(blocks, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
		rest = bytes.TrimSpace(next)
	}
	if len(certs) == 0 {
		return nil, nil, ErrMaterial
	}
	return certs, blocks, nil
}
func CSR(raw []byte) (*x509.CertificateRequest, error) {
	if len(raw) == 0 || len(raw) > MaxMaterialBytes {
		return nil, ErrMaterial
	}
	rest := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE REQUEST-----")) && !bytes.HasPrefix(rest, []byte("-----BEGIN NEW CERTIFICATE REQUEST-----")) {
		return nil, ErrMaterial
	}
	b, next := pemutil.Decode(rest)
	if b == nil || (b.Type != "CERTIFICATE REQUEST" && b.Type != "NEW CERTIFICATE REQUEST") || len(b.Headers) > 0 || len(bytes.TrimSpace(next)) > 0 {
		return nil, ErrMaterial
	}
	csr, e := x509.ParseCertificateRequest(b.Bytes)
	if e != nil || csr.CheckSignature() != nil {
		return nil, ErrMaterial
	}
	return csr, nil
}
func PrivateKey(raw []byte) (crypto.Signer, error) {
	if len(raw) == 0 || len(raw) > MaxMaterialBytes {
		return nil, ErrMaterial
	}
	rest := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(rest, []byte("-----BEGIN PRIVATE KEY-----")) {
		return nil, ErrMaterial
	}
	b, next := pemutil.Decode(rest)
	if b == nil || b.Type != "PRIVATE KEY" || len(b.Headers) > 0 || len(bytes.TrimSpace(next)) > 0 {
		return nil, ErrMaterial
	}
	defer clear(b.Bytes)
	k, e := x509.ParsePKCS8PrivateKey(b.Bytes)
	if e != nil {
		return nil, ErrMaterial
	}
	s, ok := k.(crypto.Signer)
	if !ok {
		return nil, ErrMaterial
	}
	return s, nil
}
func SamePublic(a, b any) bool {
	x, e := x509.MarshalPKIXPublicKey(a)
	if e != nil {
		return false
	}
	y, e := x509.MarshalPKIXPublicKey(b)
	return e == nil && bytes.Equal(x, y)
}
func Serial(n *big.Int) string {
	if n == nil || n.Sign() <= 0 {
		return ""
	}
	raw := hex.EncodeToString(n.Bytes())
	parts := make([]string, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":")
}
func CanonicalSerial(s string) (string, error) {
	if len(s) == 0 || len(s) > 128 || strings.TrimSpace(s) != s {
		return "", ErrMaterial
	}
	separator := ""
	if strings.Contains(s, ":") {
		separator = ":"
	}
	if strings.Contains(s, "-") {
		if separator != "" {
			return "", ErrMaterial
		}
		separator = "-"
	}
	raw := s
	if separator != "" {
		parts := strings.Split(s, separator)
		for _, p := range parts {
			if len(p) != 2 {
				return "", ErrMaterial
			}
		}
		raw = strings.Join(parts, "")
	}
	if len(raw)%2 != 0 {
		raw = "0" + raw
	}
	if len(raw) > 40 {
		return "", ErrMaterial
	}
	b, e := hex.DecodeString(raw)
	if e != nil {
		return "", ErrMaterial
	}
	n := new(big.Int).SetBytes(b)
	if n.Sign() <= 0 {
		return "", ErrMaterial
	}
	return Serial(n), nil
}
