package bao

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
	"github.com/RockInMars/openbao-sdk-go/pki"
)

func TestPKIIssueRejectsIncompleteMaterialWithoutReissue(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	cases := []struct {
		name, field string
		value       any
	}{
		{"certificate missing", "certificate", nil}, {"certificate damaged", "certificate", "broken"},
		{"serial missing", "serial_number", nil}, {"serial malformed", "serial_number", "gg"}, {"serial mismatch", "serial_number", "02:02"},
		{"issuer missing", "issuing_ca", nil}, {"issuer damaged", "issuing_ca", "broken"}, {"issuer multiple", "issuing_ca", string(f.CAPEM) + string(f.CAPEM)},
		{"chain type", "ca_chain", "wrong"}, {"chain member type", "ca_chain", []any{1}}, {"chain damaged", "ca_chain", []any{"broken"}},
		{"expiration type", "expiration", "tomorrow"}, {"expiration mismatch", "expiration", 1},
		{"key missing", "private_key", nil}, {"key damaged", "private_key", "broken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var envelope map[string]any
			if err := json.Unmarshal(pkiReply(f, true), &envelope); err != nil {
				t.Fatal(err)
			}
			envelope["data"].(map[string]any)[tc.field] = tc.value
			body, _ := json.Marshal(envelope)
			var sent atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { sent.Add(1); w.Write(body) }))
			p, _ := c.PKI("pki")
			result, err := p.Issue(context.Background(), pki.IssueRequest{Role: "device", CommonName: "terminal.test", TTL: time.Hour})
			if result != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || !baoerr.HasUnknownOutcome(err) || sent.Load() != 1 {
				t.Fatalf("invalid issuance accepted or retried: %v", err)
			}
		})
	}
}

func TestPKIIssueInvalidNamesNeverSend(t *testing.T) {
	cases := []pki.IssueRequest{
		{Role: "../bad", CommonName: "valid", TTL: time.Hour}, {Role: "device", TTL: time.Hour},
		{Role: "device", CommonName: "valid", TTL: -1}, {Role: "device", CommonName: strings.Repeat("a", 1025), TTL: time.Hour},
		{Role: "device", CommonName: "bad\nname", TTL: time.Hour}, {Role: "device", CommonName: string([]byte{255}), TTL: time.Hour},
		{Role: "device", DNSNames: []string{"bad,name"}, TTL: time.Hour}, {Role: "device", EmailNames: []string{"Display <a@example.org>"}, TTL: time.Hour},
		{Role: "device", IPAddresses: []netip.Addr{{}}, TTL: time.Hour}, {Role: "device", IPAddresses: []netip.Addr{netip.MustParseAddr("fe80::1%eth0")}, TTL: time.Hour},
		{Role: "device", URISANs: []string{"relative"}, TTL: time.Hour},
	}
	var sent atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { sent.Add(1) }))
	p, _ := c.PKI("pki")
	for i, req := range cases {
		if got, err := p.Issue(context.Background(), req); got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	if sent.Load() != 0 {
		t.Fatal("invalid issuance reached server")
	}
}

func TestPKINamesMustMatchEveryRequestedSAN(t *testing.T) {
	u, _ := url.Parse("spiffe://example.org/device")
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "device"}, DNSNames: []string{"DEVICE.example"}, EmailAddresses: []string{"a@example.org"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, URIs: []*url.URL{u}}
	good := pki.IssueRequest{CommonName: "device", DNSNames: []string{"device.example"}, EmailNames: []string{"a@example.org"}, IPAddresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")}, URISANs: []string{u.String()}}
	if !issueNamesMatch(cert, good) {
		t.Fatal("matching names rejected")
	}
	for _, req := range []pki.IssueRequest{{CommonName: "other"}, {DNSNames: []string{"other"}}, {EmailNames: []string{"b@example.org"}}, {IPAddresses: []netip.Addr{netip.MustParseAddr("127.0.0.2")}}, {URISANs: []string{"spiffe://example.org/other"}}} {
		if issueNamesMatch(cert, req) {
			t.Fatal("missing requested identity accepted")
		}
	}
}

func TestPKIMissingCertificateDoesNotReadPrivateKey(t *testing.T) {
	var sent atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		if r.Method != "GET" || r.URL.Path != "/v1/pki/cert/01-01" {
			t.Error("unexpected lookup or key request")
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"errors":[]}`))
	}))
	p, _ := c.PKI("pki")
	got, err := p.ReadCertificate(context.Background(), "01:01")
	if got != nil || !baoerr.IsCode(err, baoerr.CodeNotFoundOrHidden) || sent.Load() != 1 {
		t.Fatalf("missing certificate misclassified: %v", err)
	}
}
