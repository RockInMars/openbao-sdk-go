package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"
)

// PKIFixture is generated per test. No private PEM fixture is checked in.
type PKIFixture struct {
	CA, Leaf                                     *x509.Certificate
	Key                                          *ecdsa.PrivateKey
	CAPEM, CertificatePEM, PrivateKeyPEM, CSRPEM []byte
	Now                                          time.Time
}

func NewPKIFixture(t testing.TB) *PKIFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	caKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(101), Subject: pkix.Name{CommonName: "SDK temporary test root"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	rootDER, e := x509.CreateCertificate(rand.Reader, root, root, &caKey.PublicKey, caKey)
	if e != nil {
		t.Fatal(e)
	}
	ca, e := x509.ParseCertificate(rootDER)
	if e != nil {
		t.Fatal(e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	uri, _ := url.Parse("spiffe://fixture/terminal/t-1")
	leaf := &x509.Certificate{SerialNumber: big.NewInt(257), Subject: pkix.Name{CommonName: "terminal.test"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), DNSNames: []string{"terminal.test"}, URIs: []*url.URL{uri}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	private, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	csr, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: leaf.Subject, DNSNames: leaf.DNSNames, URIs: leaf.URIs}, key)
	if e != nil {
		t.Fatal(e)
	}
	return &PKIFixture{CA: ca, Leaf: cert, Key: key, CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), CertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), PrivateKeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}), Now: now}
}
