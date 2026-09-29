package bao

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/internal/transitutil"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"git.example.com/infra/openbao-sdk-go/transit"
)

func (t *TransitClient) Sign(ctx context.Context, r transit.SignRequest) (*transit.SignResult, error) {
	return t.sign(ctx, r.KeyName, r.KeyVersion, r.Profile, r.Message, false)
}
func (t *TransitClient) SignDigest(ctx context.Context, r transit.SignDigestRequest) (*transit.SignResult, error) {
	return t.sign(ctx, r.KeyName, r.KeyVersion, r.Profile, r.Digest, true)
}
func (t *TransitClient) sign(parent context.Context, name string, v int, p transit.Profile, value sensitive.Bytes, digest bool) (*transit.SignResult, error) {
	op := engine.TransitSign
	if digest {
		op = engine.TransitSignDigest
	}
	input := value.RevealCopy()
	defer clear(input)
	if engine.ValidateSegment(name) != nil || v <= 0 || !transitutil.ProfileValid(p, digest) || digest && len(input) != 32 || int64(len(input)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	k, pub, e := t.public(ctx, name, v)
	if e != nil {
		return nil, e
	}
	if !transitutil.Compatible(k.KeyType, p) {
		return nil, invalid(string(op))
	}
	b := transitutil.WireProfile(p, digest)
	b["key_version"] = v
	b["input"] = base64.StdEncoding.EncodeToString(input)
	payload, e := json.Marshal(b)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var out *transit.SignResult
	e = t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/sign/" + name, Payload: payload}, t.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		s, ok := d["signature"].(string)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		actual, signature, e := transitutil.Unwrap(s, int(t.client.cfg.Limits.MaxResponseBytes))
		defer clear(signature)
		if e != nil || actual != v || !transitutil.VerifyLocal(pub, p, input, digest, signature) {
			return engine.InvalidResponse(r, op)
		}
		if field, exists := d["key_version"]; exists {
			n, ok := engine.Int(field)
			if !ok || n != v {
				return engine.InvalidResponse(r, op)
			}
		}
		out = &transit.SignResult{Key: t.ref(name, v), Signature: transit.Signature{Wrapped: s, Version: v, Profile: p}, RequestID: r.RequestID}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (t *TransitClient) Verify(ctx context.Context, r transit.VerifyRequest) (*transit.VerifyResult, error) {
	return t.verify(ctx, r.KeyName, r.ExpectedVersion, r.Profile, r.Message, r.Signature, false)
}
func (t *TransitClient) VerifyDigest(ctx context.Context, r transit.VerifyDigestRequest) (*transit.VerifyResult, error) {
	return t.verify(ctx, r.KeyName, r.ExpectedVersion, r.Profile, r.Digest, r.Signature, true)
}
func (t *TransitClient) verify(parent context.Context, name string, v int, p transit.Profile, value sensitive.Bytes, s transit.Signature, digest bool) (*transit.VerifyResult, error) {
	op := engine.TransitVerify
	if digest {
		op = engine.TransitVerifyDigest
	}
	input := value.RevealCopy()
	defer clear(input)
	actual, sig, e := transitutil.Unwrap(s.Wrapped, int(t.client.cfg.Limits.MaxRequestBytes))
	defer clear(sig)
	if engine.ValidateSegment(name) != nil || v <= 0 || !transitutil.ProfileValid(p, digest) || digest && len(input) != 32 || int64(len(input)) > t.client.cfg.Limits.MaxRequestBytes || e != nil || actual != v || s.Version != v || s.Profile != p || !transitutil.SignatureShape(p, sig) {
		return nil, invalid(string(op))
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	k, _, e := t.public(ctx, name, v)
	if e != nil {
		return nil, e
	}
	if !transitutil.Compatible(k.KeyType, p) {
		return nil, invalid(string(op))
	}
	b := transitutil.WireProfile(p, digest)
	b["input"] = base64.StdEncoding.EncodeToString(input)
	b["signature"] = s.Wrapped
	return t.verifyPayload(ctx, name, "", b, op)
}
func (t *TransitClient) verifyPayload(ctx context.Context, name, suffix string, b map[string]any, op engine.Operation) (*transit.VerifyResult, error) {
	payload, e := json.Marshal(b)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var out *transit.VerifyResult
	e = t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/verify/" + name + suffix, Payload: payload}, t.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		v, ok := d["valid"].(bool)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		out = &transit.VerifyResult{Valid: v, RequestID: r.RequestID}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
