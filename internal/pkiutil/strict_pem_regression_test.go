package pkiutil_test

import (
	"bytes"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/internal/pkiutil"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
)

func malformedPEMThenValid(kind string, valid []byte) []byte {
	prefix := []byte("-----BEGIN " + kind + "-----\n!\n-----END " + kind + "-----\n")
	return append(prefix, valid...)
}

func TestPEMRejectsSkippedMalformedBlocks(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	t.Run("certificate-first", func(t *testing.T) {
		if _, _, err := pkiutil.Certificates(malformedPEMThenValid("CERTIFICATE", f.CAPEM)); err == nil {
			t.Fatal("malformed leading certificate was silently skipped")
		}
	})
	t.Run("certificate-middle", func(t *testing.T) {
		material := append(bytes.Clone(f.CertificatePEM), malformedPEMThenValid("CERTIFICATE", f.CAPEM)...)
		if _, _, err := pkiutil.Certificates(material); err == nil {
			t.Fatal("malformed middle certificate was silently skipped")
		}
	})
	t.Run("csr", func(t *testing.T) {
		if _, err := pkiutil.CSR(malformedPEMThenValid("CERTIFICATE REQUEST", f.CSRPEM)); err == nil {
			t.Fatal("malformed leading CSR was silently skipped")
		}
	})
	t.Run("private-key", func(t *testing.T) {
		material := malformedPEMThenValid("PRIVATE KEY", f.PrivateKeyPEM)
		defer clear(material)
		if _, err := pkiutil.PrivateKey(material); err == nil {
			t.Fatal("malformed leading private key was silently skipped")
		}
	})
}
