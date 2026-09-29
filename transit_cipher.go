package bao

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/internal/transitutil"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"git.example.com/infra/openbao-sdk-go/transit"
)

func (t *TransitClient) cryptoKey(ctx context.Context, name string, v int, contextBytes []byte, encrypt bool, op engine.Operation) (*keyData, error) {
	d, e := t.readKey(ctx, name, engine.TransitMetadata)
	if e != nil {
		return nil, e
	}
	if _, ok := d.keys[strconv.Itoa(v)]; !ok || v <= 0 || v > d.metadata.LatestVersion || encrypt && v < d.metadata.MinEncryptionVersion || !encrypt && v < d.metadata.MinDecryptionVersion {
		return nil, engine.SafeError(baoerr.CodeVersionUnavailable, op, 0, baoerr.EffectNone)
	}
	if !d.metadata.SupportsEncryption || (d.metadata.Type != "aes128-gcm96" && d.metadata.Type != "aes256-gcm96") || d.convergent || d.derived != (len(contextBytes) > 0) {
		return nil, invalid(string(op))
	}
	return d, nil
}
func (t *TransitClient) ciphertext(c transit.Ciphertext, op engine.Operation) error {
	v, b, e := transitutil.Unwrap(c.Wrapped, int(t.client.cfg.Limits.MaxRequestBytes))
	defer clear(b)
	if e != nil || v != c.Version || len(b) < 28 {
		return invalid(string(op))
	}
	return nil
}
func (t *TransitClient) Encrypt(parent context.Context, r transit.EncryptRequest) (*transit.CipherResult, error) {
	op := engine.TransitEncrypt
	plain := r.Plaintext.RevealCopy()
	defer clear(plain)
	contextBytes := r.Context.RevealCopy()
	defer clear(contextBytes)
	if engine.ValidateSegment(r.KeyName) != nil || r.KeyVersion <= 0 || int64(len(plain))+int64(len(contextBytes)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if _, e = t.cryptoKey(ctx, r.KeyName, r.KeyVersion, contextBytes, true, op); e != nil {
		return nil, e
	}
	b := map[string]any{"key_version": r.KeyVersion, "plaintext": base64.StdEncoding.EncodeToString(plain)}
	withDerivation(b, contextBytes)
	return t.cipherResult(ctx, r.KeyName, r.KeyVersion, "encrypt", b, op)
}
func (t *TransitClient) Decrypt(parent context.Context, r transit.DecryptRequest) (*transit.DecryptResult, error) {
	op := engine.TransitDecrypt
	cb := r.Context.RevealCopy()
	defer clear(cb)
	if engine.ValidateSegment(r.KeyName) != nil || int64(len(cb)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	if e := t.ciphertext(r.Ciphertext, op); e != nil {
		return nil, e
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if _, e = t.cryptoKey(ctx, r.KeyName, r.Ciphertext.Version, cb, false, op); e != nil {
		return nil, e
	}
	b := map[string]any{"ciphertext": r.Ciphertext.Wrapped}
	withDerivation(b, cb)
	payload, e := json.Marshal(b)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var out *transit.DecryptResult
	e = t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/decrypt/" + r.KeyName, Payload: payload}, t.mount, func(resp *engine.Response) error {
		d, e := engine.Data(resp, op)
		if e != nil {
			return e
		}
		s, ok := d["plaintext"].(string)
		if !ok {
			return engine.InvalidResponse(resp, op)
		}
		raw, e := base64.StdEncoding.Strict().DecodeString(s)
		defer clear(raw)
		if e != nil || base64.StdEncoding.EncodeToString(raw) != s {
			return engine.InvalidResponse(resp, op)
		}
		out = &transit.DecryptResult{Plaintext: sensitive.NewBytes(raw), RequestID: resp.RequestID}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (t *TransitClient) Rewrap(parent context.Context, r transit.RewrapRequest) (*transit.CipherResult, error) {
	op := engine.TransitRewrap
	cb := r.Context.RevealCopy()
	defer clear(cb)
	if engine.ValidateSegment(r.KeyName) != nil || r.TargetVersion <= 0 || int64(len(cb)) > t.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	if e := t.ciphertext(r.Ciphertext, op); e != nil {
		return nil, e
	}
	ctx, cancel, e := t.operationContext(parent, op)
	if e != nil {
		return nil, e
	}
	defer cancel()
	d, e := t.cryptoKey(ctx, r.KeyName, r.TargetVersion, cb, true, op)
	if e != nil {
		return nil, e
	}
	if _, ok := d.keys[strconv.Itoa(r.Ciphertext.Version)]; !ok || r.Ciphertext.Version < d.metadata.MinDecryptionVersion {
		return nil, engine.SafeError(baoerr.CodeVersionUnavailable, op, 0, baoerr.EffectNone)
	}
	b := map[string]any{"key_version": r.TargetVersion, "ciphertext": r.Ciphertext.Wrapped}
	withDerivation(b, cb)
	return t.cipherResult(ctx, r.KeyName, r.TargetVersion, "rewrap", b, op)
}
func withDerivation(b map[string]any, cb []byte) {
	if len(cb) > 0 {
		b["context"] = base64.StdEncoding.EncodeToString(cb)
	}
}
func (t *TransitClient) cipherResult(ctx context.Context, name string, v int, action string, b map[string]any, op engine.Operation) (*transit.CipherResult, error) {
	payload, e := json.Marshal(b)
	if e != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var out *transit.CipherResult
	e = t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/" + action + "/" + name, Payload: payload}, t.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		s, ok := d["ciphertext"].(string)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		actual, blob, e := transitutil.Unwrap(s, int(t.client.cfg.Limits.MaxResponseBytes))
		defer clear(blob)
		if e != nil || actual != v || len(blob) < 28 {
			return engine.InvalidResponse(r, op)
		}
		if field, exists := d["key_version"]; exists {
			n, ok := engine.Int(field)
			if !ok || n != v {
				return engine.InvalidResponse(r, op)
			}
		}
		out = &transit.CipherResult{Ciphertext: transit.Ciphertext{Wrapped: s, Version: v}, RequestID: r.RequestID}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
