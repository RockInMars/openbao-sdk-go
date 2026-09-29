package bao

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/internal/transitutil"
	"git.example.com/infra/openbao-sdk-go/transit"
)

func (t *TransitClient) hmacKey(ctx context.Context, name string, v int, sign bool, op engine.Operation) error {
	d, e := t.readKey(ctx, name, engine.TransitMetadata)
	if e != nil {
		return e
	}
	if _, ok := d.keys[strconv.Itoa(v)]; !ok || v <= 0 || sign && v < d.metadata.MinEncryptionVersion || !sign && v < d.metadata.MinDecryptionVersion {
		return engine.SafeError(baoerr.CodeVersionUnavailable, op, 0, baoerr.EffectNone)
	}
	// This V1 contract has no derivation-context field for HMAC.
	if d.derived {
		return invalid(string(op))
	}
	return nil
}
func (t *TransitClient) HMAC(parent context.Context, r transit.HMACRequest) (*transit.HMACResult, error) {
	op := engine.TransitHMAC
	raw := r.Message.RevealCopy()
	defer clear(raw)
	if engine.ValidateSegment(r.KeyName) != nil || r.KeyVersion <= 0 || int64(len(raw)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if e = t.hmacKey(ctx, r.KeyName, r.KeyVersion, true, op); e != nil {
		return nil, e
	}
	payload, e := json.Marshal(map[string]any{"input": base64.StdEncoding.EncodeToString(raw), "key_version": r.KeyVersion})
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var out *transit.HMACResult
	e = t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/hmac/" + r.KeyName + "/sha2-256", Payload: payload}, t.mount, func(resp *engine.Response) error {
		d, e := engine.Data(resp, op)
		if e != nil {
			return e
		}
		s, ok := d["hmac"].(string)
		if !ok {
			return engine.InvalidResponse(resp, op)
		}
		v, mac, e := transitutil.Unwrap(s, int(t.client.cfg.Limits.MaxResponseBytes))
		defer clear(mac)
		if e != nil || v != r.KeyVersion || len(mac) != 32 {
			return engine.InvalidResponse(resp, op)
		}
		out = &transit.HMACResult{Wrapped: s, Version: v, RequestID: resp.RequestID}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (t *TransitClient) HMACVerify(parent context.Context, r transit.HMACVerifyRequest) (*transit.VerifyResult, error) {
	op := engine.TransitHMACVerify
	raw := r.Message.RevealCopy()
	defer clear(raw)
	v, mac, e := transitutil.Unwrap(r.WrappedHMAC, int(t.client.cfg.Limits.MaxRequestBytes))
	defer clear(mac)
	if engine.ValidateSegment(r.KeyName) != nil || r.ExpectedVersion <= 0 || int64(len(raw)) > t.client.cfg.Limits.MaxRequestBytes || e != nil || v != r.ExpectedVersion || len(mac) != 32 {
		return nil, invalid(string(op))
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if e = t.hmacKey(ctx, r.KeyName, r.ExpectedVersion, false, op); e != nil {
		return nil, e
	}
	return t.verifyPayload(ctx, r.KeyName, "/sha2-256", map[string]any{"input": base64.StdEncoding.EncodeToString(raw), "hmac": r.WrappedHMAC}, op)
}
