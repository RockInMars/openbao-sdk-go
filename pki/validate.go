package pki

import (
	"crypto/x509"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/pkiutil"
	"strings"
	"time"
)

func validationError() error {
	return &baoerr.Error{Code: baoerr.CodeInvalidArgument, Operation: "PKI_VALIDATE", Effect: baoerr.EffectNone, Message: "certificate validation failed"}
}

// ValidateCSR checks PEM framing, ASN.1 structure and the CSR's own signature.
func ValidateCSR(raw []byte) error {
	if _, e := pkiutil.CSR(raw); e != nil {
		return validationError()
	}
	return nil
}

// ValidateBundle performs offline validation. Only caller-supplied roots are
// trust anchors; certificates attached to the response remain intermediates.
func ValidateBundle(bundle IssuedCertificate, policy VerifyPolicy) error {
	fail := validationError
	if len(policy.TrustedRootsPEM) == 0 || len(policy.TrustedRootsPEM) > 64 || len(policy.RequiredEKUs) == 0 || len(policy.RequiredEKUs) > 16 || policy.MinRemainingTTL < 0 {
		return fail()
	}
	certs, _, e := pkiutil.Certificates(bundle.Certificate.CertificatePEM)
	if e != nil {
		return fail()
	}
	leaf := certs[0]
	key := bundle.PrivateKey.RevealCopy()
	defer clear(key)
	private, e := pkiutil.PrivateKey(key)
	if e != nil || !pkiutil.SamePublic(private.Public(), leaf.PublicKey) {
		return fail()
	}
	roots := x509.NewCertPool()
	for _, raw := range policy.TrustedRootsPEM {
		cs, _, e := pkiutil.Certificates(raw)
		if e != nil {
			return fail()
		}
		for _, c := range cs {
			if !c.IsCA {
				return fail()
			}
			roots.AddCert(c)
		}
	}
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	materials := append([][]byte(nil), bundle.Certificate.CAChainPEM...)
	if len(bundle.Certificate.IssuingCAPEM) > 0 {
		materials = append(materials, bundle.Certificate.IssuingCAPEM)
	}
	if len(materials) > 64 {
		return fail()
	}
	for _, raw := range materials {
		cs, _, e := pkiutil.Certificates(raw)
		if e != nil {
			return fail()
		}
		for _, c := range cs {
			intermediates.AddCert(c)
		}
	}
	now := policy.CurrentTime
	if now.IsZero() {
		now = time.Now()
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) || leaf.NotAfter.Sub(now) < policy.MinRemainingTTL {
		return fail()
	}
	if !bundle.Certificate.NotBefore.IsZero() && !bundle.Certificate.NotBefore.Equal(leaf.NotBefore) || !bundle.Certificate.NotAfter.IsZero() && !bundle.Certificate.NotAfter.Equal(leaf.NotAfter) {
		return fail()
	}
	if bundle.Certificate.SerialNumber != "" {
		serial, e := pkiutil.CanonicalSerial(bundle.Certificate.SerialNumber)
		if e != nil || serial != pkiutil.Serial(leaf.SerialNumber) {
			return fail()
		}
	}
	if policy.ExpectedCN != "" && leaf.Subject.CommonName != policy.ExpectedCN {
		return fail()
	}
	for _, want := range policy.ExpectedDNS {
		found := false
		for _, actual := range leaf.DNSNames {
			if strings.EqualFold(actual, want) {
				found = true
			}
		}
		if !found {
			return fail()
		}
	}
	for _, want := range policy.ExpectedURI {
		found := false
		for _, actual := range leaf.URIs {
			if actual.String() == want {
				found = true
			}
		}
		if !found {
			return fail()
		}
	}
	for _, usage := range policy.RequiredEKUs {
		if _, e := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{usage}}); e != nil {
			return fail()
		}
	}
	return nil
}
