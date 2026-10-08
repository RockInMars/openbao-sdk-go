package bao

import (
	"context"
	"encoding/json"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPKIZeroTTLRejected(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		jsonData(w, map[string]any{"certificate": string(f.CertificatePEM), "private_key": string(f.PrivateKeyPEM), "issuing_ca": string(f.CAPEM), "serial_number": "01:01", "expiration": f.Leaf.NotAfter.Unix()})
	}))
	p, _ := c.PKI("pki")
	if _, e := p.Issue(context.Background(), pki.IssueRequest{Role: "terminal", CommonName: "terminal.test"}); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("Issue zero TTL did not fail locally")
	}
	if _, e := p.SignCSR(context.Background(), pki.SignCSRRequest{Role: "terminal", CSRPEM: sensitive.NewBytes(f.CSRPEM)}); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("SignCSR zero TTL did not fail locally")
	}
	if n.Load() != 0 {
		t.Fatal("zero TTL reached service")
	}
}
func pkiReply(f *testutil.PKIFixture, key bool) []byte {
	m := map[string]any{"certificate": string(f.CertificatePEM), "issuing_ca": string(f.CAPEM), "ca_chain": []string{string(f.CAPEM)}, "serial_number": "01:01", "expiration": f.Leaf.NotAfter.Unix()}
	if key {
		m["private_key"] = string(f.PrivateKeyPEM)
	}
	b, _ := json.Marshal(map[string]any{"data": m})
	return b
}
func TestPKIIssueContract(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Path != "/v1/pki-device/issue/terminal" {
			t.Error("Issue unexpected route (including KV)")
		}
		var m map[string]any
		if json.NewDecoder(r.Body).Decode(&m) != nil || m["format"] != "pem" || m["private_key_format"] != "pkcs8" || m["key_type"] != nil || m["key_bits"] != nil {
			t.Error("Issue parameters wrong")
		}
		w.Write(pkiReply(f, true))
	}))
	p, e := c.PKI("pki-device")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Issue(context.Background(), pki.IssueRequest{Role: "terminal", CommonName: "terminal.test", DNSNames: []string{"terminal.test"}, TTL: time.Hour})
	if e != nil || r.Certificate.SerialNumber != "01:01" || r.PrivateKey.Len() == 0 || n.Load() != 1 {
		t.Fatal("Issue result invalid")
	}
	r.PrivateKey.Zero()
}
func TestPKIPrivateKeyMismatch(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	f.PrivateKeyPEM = testutil.NewPKIFixture(t).PrivateKeyPEM
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.Write(pkiReply(f, true)) }))
	p, _ := c.PKI("pki")
	r, e := p.Issue(context.Background(), pki.IssueRequest{Role: "terminal", CommonName: "terminal.test", TTL: time.Hour})
	if r != nil || !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || !baoerr.HasUnknownOutcome(e) || n.Load() != 1 {
		t.Fatal("bad key accepted or reissued")
	}
}
func TestPKICSRPublicKeyMatch(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	other := testutil.NewPKIFixture(t)
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Path != "/v1/pki/sign/terminal" {
			t.Error("CSR route")
		}
		w.Write(pkiReply(other, false))
	}))
	p, _ := c.PKI("pki")
	r, e := p.SignCSR(context.Background(), pki.SignCSRRequest{Role: "terminal", CSRPEM: sensitive.NewBytes(f.CSRPEM), TTL: time.Hour})
	if r != nil || !baoerr.HasUnknownOutcome(e) || n.Load() != 1 {
		t.Fatal("CSR wrong key accepted")
	}
	if _, e = p.SignCSR(context.Background(), pki.SignCSRRequest{Role: "terminal", CSRPEM: sensitive.NewBytes([]byte("broken"))}); e == nil || n.Load() != 1 {
		t.Fatal("broken CSR sent")
	}
}
func TestPKIReadMultiPEMAndChain(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "ca_chain") {
			w.Write(f.CAPEM)
			return
		}
		b, _ := json.Marshal(map[string]any{"data": map[string]any{"certificate": string(append(append([]byte{}, f.CertificatePEM...), f.CAPEM...)), "revocation_time": 0}})
		w.Write(b)
	}))
	p, _ := c.PKI("pki")
	r, e := p.ReadCertificate(context.Background(), "01:01")
	if e != nil || r.SerialNumber != "01:01" || r.RevokedAt != nil || strings.Count(string(r.CertificatePEM), "BEGIN CERTIFICATE") != 1 {
		t.Fatal("certificate record parse")
	}
	chain, e := p.ReadIssuerChain(context.Background())
	if e != nil || len(chain.CertificatesPEM) != 1 {
		t.Fatal("raw chain parse")
	}
}
func TestPKIRevokeReceipt(t *testing.T) {
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Write([]byte(`{"data":{}}`))
			return
		}
		w.Write([]byte(`{"data":{"revocation_time":1700000000}}`))
	}))
	p, _ := c.PKI("pki")
	if r, e := p.Revoke(context.Background(), "01:01"); r != nil || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("revocation time fabricated")
	}
	r, e := p.Revoke(context.Background(), "01:01")
	if e != nil || r.RevokedAt.Unix() != 1700000000 {
		t.Fatal("server receipt ignored")
	}
}
