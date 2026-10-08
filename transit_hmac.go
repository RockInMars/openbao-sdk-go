package bao

import (
	"context"
	"encoding/base64"
	"strconv"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/transitutil"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func (t *TransitClient) hmacKey(ctx context.Context, name string, v int, sign bool, op engine.Operation) error {
	d, e := t.readKey(ctx, name, engine.TransitMetadata)
	if e != nil {
		return e
	}
	_, listed := d.keys[strconv.Itoa(v)]
	if !listed && d.metadata.Type == "hmac" && len(d.keys) == 0 {
		// Without a version list, only the reported current version is known.
		listed = v == d.metadata.LatestVersion
	}
	if !listed || v <= 0 || sign && v < d.metadata.MinEncryptionVersion || !sign && v < d.metadata.MinDecryptionVersion {
		return engine.SafeError(baoerr.CodeVersionUnavailable, op, 0, baoerr.EffectNone)
	}
	// This V1 contract has no derivation-context field for HMAC.
	if d.derived {
		return invalid(string(op))
	}
	return nil
}

// HMAC computes SHA-256 HMAC using explicit KeyVersion. Message remains
// caller-owned. Grant key metadata read and hmac permission; missing version
// metadata is handled conservatively, not as proof of historical key availability.
func (t *TransitClient) HMAC(parent context.Context, r transit.HMACRequest) (*transit.HMACResult, error) {
	op := engine.TransitHMAC
	raw := r.Message.RevealCopy()
	defer clear(raw)
	if engine.ValidateSegment(r.KeyName) != nil || r.KeyVersion <= 0 || int64(len(raw)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	payload, e := t.requestPayload(map[string]any{"input": base64.StdEncoding.EncodeToString(raw), "key_version": r.KeyVersion}, op)
	if e != nil {
		return nil, e
	}
	defer clear(payload)
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if e = t.hmacKey(ctx, r.KeyName, r.KeyVersion, true, op); e != nil {
		return nil, e
	}
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

// HMACVerify verifies a wrapped SHA-256 HMAC for ExpectedVersion. A successful
// response with Valid=false is distinct from a transport or permission error.
func (t *TransitClient) HMACVerify(parent context.Context, r transit.HMACVerifyRequest) (*transit.VerifyResult, error) {
	op := engine.TransitHMACVerify
	raw := r.Message.RevealCopy()
	defer clear(raw)
	v, mac, e := transitutil.Unwrap(r.WrappedHMAC, int(t.client.cfg.Limits.MaxRequestBytes))
	defer clear(mac)
	if engine.ValidateSegment(r.KeyName) != nil || r.ExpectedVersion <= 0 || int64(len(raw)) > t.client.cfg.Limits.MaxRequestBytes || e != nil || v != r.ExpectedVersion || len(mac) != 32 {
		return nil, invalid(string(op))
	}
	payload, e := t.requestPayload(map[string]any{"input": base64.StdEncoding.EncodeToString(raw), "hmac": r.WrappedHMAC}, op)
	if e != nil {
		return nil, e
	}
	defer clear(payload)
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if e = t.hmacKey(ctx, r.KeyName, r.ExpectedVersion, false, op); e != nil {
		return nil, e
	}
	return t.verifyPayload(ctx, r.KeyName, "/sha2-256", payload, op)
}
