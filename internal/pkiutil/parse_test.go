package pkiutil_test

import (
	"bytes"
	"encoding/pem"
	"git.example.com/infra/openbao-sdk-go/internal/pkiutil"
	"git.example.com/infra/openbao-sdk-go/internal/testutil"
	"math/big"
	"strings"
	"testing"
)

func TestCertificateMaterialBoundaries(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	for _, raw := range [][]byte{nil, []byte("prefix"), []byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid ASN1")}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"Header": "unsupported"}, Bytes: f.Leaf.Raw}), append([]byte("junk"), f.CAPEM...), append(append([]byte(nil), f.CAPEM...), []byte("junk")...), bytes.Repeat(f.CAPEM, 65), make([]byte, pkiutil.MaxMaterialBytes+1)} {
		if _, _, e := pkiutil.Certificates(raw); e == nil {
			t.Fatal("invalid certificate material accepted")
		}
	}
	certs, blocks, e := pkiutil.Certificates(append(append([]byte(nil), f.CertificatePEM...), f.CAPEM...))
	if e != nil || len(certs) != 2 || len(blocks) != 2 {
		t.Fatal("valid certificate bundle rejected")
	}
}
func TestPrivateKeyMaterialBoundaries(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	key, e := pkiutil.PrivateKey(f.PrivateKeyPEM)
	if e != nil || !pkiutil.SamePublic(key.Public(), f.Leaf.PublicKey) {
		t.Fatal("valid key rejected")
	}
	for _, raw := range [][]byte{nil, make([]byte, pkiutil.MaxMaterialBytes+1), []byte("junk"), []byte("-----BEGIN PRIVATE KEY-----\n!\n-----END PRIVATE KEY-----"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("invalid")}), append(append([]byte(nil), f.PrivateKeyPEM...), f.PrivateKeyPEM...)} {
		if _, e := pkiutil.PrivateKey(raw); e == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if pkiutil.SamePublic(struct{}{}, key.Public()) || pkiutil.SamePublic(key.Public(), struct{}{}) {
		t.Fatal("invalid public key comparison")
	}
}
func TestSerialBoundaries(t *testing.T) {
	for in, want := range map[string]string{"1": "01", "01-AB": "01:ab", "01:ab": "01:ab", "0001": "01"} {
		s, e := pkiutil.CanonicalSerial(in)
		if e != nil || s != want {
			t.Fatal("valid serial rejected")
		}
	}
	for _, in := range []string{"", "0", "00", " x", "1:22", "01:02-03", "gg", strings.Repeat("1", 42), strings.Repeat("1", 129)} {
		if _, e := pkiutil.CanonicalSerial(in); e == nil {
			t.Fatal("invalid serial accepted")
		}
	}
	if pkiutil.Serial(nil) != "" || pkiutil.Serial(big.NewInt(0)) != "" {
		t.Fatal("nil/zero serial accepted")
	}
}
