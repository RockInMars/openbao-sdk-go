package bao

import (
	"context"
	"encoding/json"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/pkiutil"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"strings"
	"time"
)

// ReadCertificate reads a stored certificate by serial. RevokedAt is populated
// only when returned by the server; its absence is not proof of non-revocation.
func (p *PKIClient) ReadCertificate(ctx context.Context, serial string) (*pki.CertificateRecord, error) {
	op := engine.PKIRead
	canonical, e := pkiutil.CanonicalSerial(serial)
	if e != nil {
		return nil, invalid(string(op))
	}
	var result *pki.CertificateRecord
	err := p.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + p.mount + "/cert/" + strings.ReplaceAll(canonical, ":", "-")}, p.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		s, ok := d["certificate"].(string)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		cs, blocks, e := pkiutil.Certificates([]byte(s))
		if e != nil || pkiutil.Serial(cs[0].SerialNumber) != canonical {
			return engine.InvalidResponse(r, op)
		}
		out := &pki.CertificateRecord{CertificatePEM: blocks[0], SerialNumber: canonical, NotBefore: cs[0].NotBefore, NotAfter: cs[0].NotAfter, RequestID: r.RequestID}
		if v, exists := d["revocation_time"]; exists {
			n, ok := engine.Int(v)
			if !ok || n < 0 || int64(n) > 253402300799 {
				return engine.InvalidResponse(r, op)
			}
			if n > 0 {
				at := time.Unix(int64(n), 0).UTC()
				out.RevokedAt = &at
			}
		}
		result = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReadIssuerChain retrieves the server's chain. Returned roots are not implicitly
// trusted; verification must use trust anchors selected by the caller.
func (p *PKIClient) ReadIssuerChain(ctx context.Context) (*pki.ChainResult, error) {
	op := engine.PKIChain
	var result *pki.ChainResult
	err := p.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + p.mount + "/ca_chain"}, p.mount, func(r *engine.Response) error {
		_, blocks, e := pkiutil.Certificates(r.Body)
		if e != nil {
			return engine.InvalidResponse(r, op)
		}
		result = &pki.ChainResult{CertificatesPEM: blocks, RequestID: r.RequestID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Revoke requests revocation by serial and requires revoke permission. A lost
// response can leave the result unknown; reconciliation belongs to the caller.
func (p *PKIClient) Revoke(ctx context.Context, serial string) (*pki.RevokeResult, error) {
	op := engine.PKIRevoke
	canonical, e := pkiutil.CanonicalSerial(serial)
	if e != nil {
		return nil, invalid(string(op))
	}
	payload, e := json.Marshal(map[string]string{"serial_number": canonical})
	if e != nil {
		return nil, invalid(string(op))
	}
	var result *pki.RevokeResult
	err := p.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + p.mount + "/revoke", Payload: payload}, p.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		n, ok := engine.Int(d["revocation_time"])
		if !ok || n <= 0 || int64(n) > 253402300799 {
			return engine.InvalidResponse(r, op)
		}
		result = &pki.RevokeResult{RevokedAt: time.Unix(int64(n), 0).UTC(), RequestID: r.RequestID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
