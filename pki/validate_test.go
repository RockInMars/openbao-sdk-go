package pki

import (
	"crypto/x509"
	"encoding/pem"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"testing"
	"time"
)

func TestPKITrustAndLifetime(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	b := IssuedCertificate{Certificate: Certificate{CertificatePEM: f.CertificatePEM, IssuingCAPEM: f.CAPEM, CAChainPEM: [][]byte{f.CAPEM}, NotBefore: f.Leaf.NotBefore, NotAfter: f.Leaf.NotAfter}, PrivateKey: sensitive.NewBytes(f.PrivateKeyPEM)}
	defer b.PrivateKey.Zero()
	good := VerifyPolicy{TrustedRootsPEM: [][]byte{f.CAPEM}, CurrentTime: f.Now, RequiredEKUs: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, ExpectedCN: "terminal.test", ExpectedDNS: []string{"terminal.test"}, ExpectedURI: []string{"spiffe://fixture/terminal/t-1"}, MinRemainingTTL: 30 * time.Minute}
	if e := ValidateBundle(b, good); e != nil {
		t.Fatal(e)
	}
	cases := []VerifyPolicy{good, good, good, good, good, good, good, good}
	cases[0].TrustedRootsPEM = nil
	cases[1].RequiredEKUs = nil
	cases[2].RequiredEKUs = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	cases[3].ExpectedDNS = []string{"other.test"}
	cases[4].ExpectedURI = []string{"spiffe://other"}
	cases[5].MinRemainingTTL = 2 * time.Hour
	cases[6].CurrentTime = f.Now.Add(2 * time.Hour)
	cases[7].TrustedRootsPEM = [][]byte{testutil.NewPKIFixture(t).CAPEM}
	for i, p := range cases {
		if e := ValidateBundle(b, p); e == nil {
			t.Fatalf("policy case %d accepted", i)
		}
	}
	b.PrivateKey = sensitive.NewBytes(testutil.NewPKIFixture(t).PrivateKeyPEM)
	defer b.PrivateKey.Zero()
	if e := ValidateBundle(b, good); e == nil {
		t.Fatal("mismatched key accepted")
	}
}
func TestPKICSRSignature(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	if e := ValidateCSR(f.CSRPEM); e != nil {
		t.Fatal(e)
	}
	block, _ := pem.Decode(f.CSRPEM)
	block.Bytes[len(block.Bytes)-1] ^= 1
	bad := pem.EncodeToMemory(block)
	if e := ValidateCSR(bad); e == nil {
		t.Fatal("invalid CSR signature accepted")
	}
	if e := ValidateCSR(append(f.CSRPEM, f.CSRPEM...)); e == nil {
		t.Fatal("extra CSR block accepted")
	}
}
func FuzzPKICSR(f *testing.F) {
	f.Add([]byte("invalid fixture"))
	fixture := testutil.NewPKIFixture(f)
	f.Add(fixture.CSRPEM)
	f.Add(append([]byte("-----BEGIN CERTIFICATE REQUEST-----\n!\n-----END CERTIFICATE REQUEST-----\n"), fixture.CSRPEM...))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) <= 1<<20 {
			_ = ValidateCSR(b)
		}
	})
}

func TestPKIMetadataAndMalformedPolicy(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	base := IssuedCertificate{Certificate: Certificate{CertificatePEM: f.CertificatePEM, IssuingCAPEM: f.CAPEM, CAChainPEM: [][]byte{f.CAPEM}, NotBefore: f.Leaf.NotBefore, NotAfter: f.Leaf.NotAfter}, PrivateKey: sensitive.NewBytes(f.PrivateKeyPEM)}
	defer base.PrivateKey.Zero()
	policy := VerifyPolicy{TrustedRootsPEM: [][]byte{f.CAPEM}, RequiredEKUs: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, ExpectedCN: "terminal.test"}
	if e := ValidateBundle(base, policy); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*IssuedCertificate, *VerifyPolicy){
		func(b *IssuedCertificate, p *VerifyPolicy) { b.Certificate.CertificatePEM = []byte("invalid") },
		func(b *IssuedCertificate, p *VerifyPolicy) { p.TrustedRootsPEM = [][]byte{[]byte("invalid")} },
		func(b *IssuedCertificate, p *VerifyPolicy) { p.TrustedRootsPEM = [][]byte{f.CertificatePEM} },
		func(b *IssuedCertificate, p *VerifyPolicy) { b.Certificate.CAChainPEM = make([][]byte, 65) },
		func(b *IssuedCertificate, p *VerifyPolicy) { b.Certificate.CAChainPEM = [][]byte{[]byte("invalid")} },
		func(b *IssuedCertificate, p *VerifyPolicy) {
			b.Certificate.NotBefore = b.Certificate.NotBefore.Add(time.Second)
		},
		func(b *IssuedCertificate, p *VerifyPolicy) { b.Certificate.SerialNumber = "ab:cd" },
		func(b *IssuedCertificate, p *VerifyPolicy) { p.ExpectedCN = "other.test" },
		func(b *IssuedCertificate, p *VerifyPolicy) { p.MinRemainingTTL = -time.Second },
	} {
		b, p := base, policy
		mutate(&b, &p)
		if e := ValidateBundle(b, p); e == nil {
			t.Fatal("bad metadata/policy accepted")
		}
	}
}
