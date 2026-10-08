package bao

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/pkiutil"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

// PKIClient performs certificate operations; it never persists credentials.
type PKIClient struct {
	client *Client
	mount  string
}

// PKI binds an existing PKI mount; roles, issuers, and server policy must already
// be provisioned by an administrator. Binding performs no network I/O.
func (c *Client) PKI(mount string) (*PKIClient, error) {
	if c == nil || engine.ValidatePath(mount) != nil {
		return nil, invalid("PKI_MOUNT")
	}
	return &PKIClient{client: c, mount: mount}, nil
}
func certificateText(s string, empty bool) bool {
	if !empty && s == "" || len(s) > 1024 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Issue requests a certificate and server-generated private key from req.Role.
// The caller must Zero the returned PrivateKey after use. An unknown outcome may
// still have issued a certificate; do not blindly repeat issuance.
func (p *PKIClient) Issue(ctx context.Context, req pki.IssueRequest) (*pki.IssuedCertificate, error) {
	op := engine.PKIIssue
	if engine.ValidateSegment(req.Role) != nil || req.TTL <= 0 || !certificateText(req.CommonName, true) {
		return nil, invalid(string(op))
	}
	if req.CommonName == "" && len(req.DNSNames)+len(req.EmailNames)+len(req.IPAddresses)+len(req.URISANs) == 0 {
		return nil, invalid(string(op))
	}
	data := map[string]any{"common_name": req.CommonName, "format": "pem", "private_key_format": "pkcs8", "exclude_cn_from_sans": req.ExcludeCNFromSANs}
	if req.TTL > 0 {
		data["ttl"] = req.TTL.String()
	}
	names := append([]string(nil), req.DNSNames...)
	for _, name := range names {
		if !certificateText(name, false) || strings.ContainsAny(name, ", \t") {
			return nil, invalid(string(op))
		}
	}
	for _, name := range req.EmailNames {
		a, e := mail.ParseAddress(name)
		if e != nil || a.Address != name || !certificateText(name, false) || strings.Contains(name, ",") {
			return nil, invalid(string(op))
		}
		names = append(names, name)
	}
	if len(names) > 0 {
		data["alt_names"] = strings.Join(names, ",")
	}
	ips := make([]string, 0, len(req.IPAddresses))
	for _, ip := range req.IPAddresses {
		if !ip.IsValid() || ip.Zone() != "" {
			return nil, invalid(string(op))
		}
		ips = append(ips, ip.String())
	}
	if len(ips) > 0 {
		data["ip_sans"] = strings.Join(ips, ",")
	}
	for _, name := range req.URISANs {
		u, e := url.Parse(name)
		if e != nil || u.Scheme == "" || !certificateText(name, false) || strings.Contains(name, ",") {
			return nil, invalid(string(op))
		}
	}
	if len(req.URISANs) > 0 {
		data["uri_sans"] = strings.Join(req.URISANs, ",")
	}
	payload, e := json.Marshal(data)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var result *pki.IssuedCertificate
	err := p.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + p.mount + "/issue/" + req.Role, Payload: payload}, p.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		cert, leaf, e := issuedCertificate(d, r, op)
		if e != nil {
			return e
		}
		raw, ok := d["private_key"].(string)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		keyBytes := []byte(raw)
		defer clear(keyBytes)
		key, e := pkiutil.PrivateKey(keyBytes)
		if e != nil || !pkiutil.SamePublic(key.Public(), leaf.PublicKey) || !issueNamesMatch(leaf, req) {
			return engine.InvalidResponse(r, op)
		}
		result = &pki.IssuedCertificate{Certificate: cert, PrivateKey: sensitive.NewBytes(keyBytes), RequestID: r.RequestID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SignCSR signs a caller-created CSR using the configured server role. The
// caller keeps its private key and ownership of req.CSRPEM. Server policy decides
// which names and TTL are permitted; uncertain issuance requires reconciliation.
func (p *PKIClient) SignCSR(ctx context.Context, req pki.SignCSRRequest) (*pki.SignedCertificate, error) {
	op := engine.PKISignCSR
	if engine.ValidateSegment(req.Role) != nil || req.TTL <= 0 {
		return nil, invalid(string(op))
	}
	raw := req.CSRPEM.RevealCopy()
	defer clear(raw)
	if int64(len(raw)) > p.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	csr, e := pkiutil.CSR(raw)
	if e != nil {
		return nil, invalid(string(op))
	}
	data := map[string]any{"csr": string(raw), "format": "pem"}
	if req.TTL > 0 {
		data["ttl"] = req.TTL.String()
	}
	payload, e := json.Marshal(data)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var result *pki.SignedCertificate
	err := p.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + p.mount + "/sign/" + req.Role, Payload: payload}, p.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		cert, leaf, e := issuedCertificate(d, r, op)
		if e != nil {
			return e
		}
		if !pkiutil.SamePublic(csr.PublicKey, leaf.PublicKey) {
			return engine.InvalidResponse(r, op)
		}
		result = &pki.SignedCertificate{Certificate: cert, RequestID: r.RequestID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func issuedCertificate(d map[string]any, r *engine.Response, op engine.Operation) (pki.Certificate, *x509.Certificate, error) {
	bad := func() (pki.Certificate, *x509.Certificate, error) {
		return pki.Certificate{}, nil, engine.InvalidResponse(r, op)
	}
	text, ok := d["certificate"].(string)
	if !ok {
		return bad()
	}
	certs, blocks, e := pkiutil.Certificates([]byte(text))
	if e != nil {
		return bad()
	}
	leaf := certs[0]
	serialText, ok := d["serial_number"].(string)
	if !ok {
		return bad()
	}
	serial, e := pkiutil.CanonicalSerial(serialText)
	if e != nil || serial != pkiutil.Serial(leaf.SerialNumber) {
		return bad()
	}
	issuer, ok := d["issuing_ca"].(string)
	if !ok {
		return bad()
	}
	_, issuerBlocks, e := pkiutil.Certificates([]byte(issuer))
	if e != nil || len(issuerBlocks) != 1 {
		return bad()
	}
	out := pki.Certificate{CertificatePEM: blocks[0], SerialNumber: serial, IssuingCAPEM: issuerBlocks[0], NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter}
	hash := sha256.Sum256(leaf.Raw)
	out.FingerprintSHA256 = hex.EncodeToString(hash[:])
	out.CAChainPEM = append(out.CAChainPEM, blocks[1:]...)
	if raw, exists := d["ca_chain"]; exists {
		a, ok := raw.([]any)
		if !ok {
			return bad()
		}
		for _, v := range a {
			s, ok := v.(string)
			if !ok {
				return bad()
			}
			_, chain, e := pkiutil.Certificates([]byte(s))
			if e != nil {
				return bad()
			}
			out.CAChainPEM = append(out.CAChainPEM, chain...)
		}
	}
	if len(out.CAChainPEM) > 64 {
		return bad()
	}
	if raw, exists := d["expiration"]; exists {
		n, ok := engine.Int(raw)
		if !ok || int64(n) != leaf.NotAfter.Unix() {
			return bad()
		}
	}
	return out, leaf, nil
}
func issueNamesMatch(cert *x509.Certificate, req pki.IssueRequest) bool {
	if req.CommonName != "" && cert.Subject.CommonName != req.CommonName {
		return false
	}
	for _, want := range req.DNSNames {
		ok := false
		for _, got := range cert.DNSNames {
			if strings.EqualFold(got, want) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	for _, want := range req.EmailNames {
		ok := false
		for _, got := range cert.EmailAddresses {
			if got == want {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	for _, want := range req.IPAddresses {
		ok := false
		for _, got := range cert.IPAddresses {
			if got.Equal(want.AsSlice()) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	for _, want := range req.URISANs {
		ok := false
		for _, got := range cert.URIs {
			if got.String() == want {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
