package bao

import (
	"context"
	"crypto"
	"sort"
	"strconv"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/transitutil"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

// TransitClient binds cryptographic operations to one fixed mount. Keys must be
// provisioned independently. All metadata preflights are read-only and uncached.
type TransitClient struct {
	client *Client
	mount  string
}

func (c *Client) Transit(mount string) (*TransitClient, error) {
	if c == nil || engine.ValidatePath(mount) != nil {
		return nil, invalid("TRANSIT_MOUNT")
	}
	return &TransitClient{c, mount}, nil
}
func (t *TransitClient) ref(name string, v int) transit.KeyRef {
	return transit.KeyRef{ClusterAlias: t.client.cfg.ClusterAlias, Namespace: t.client.cfg.Namespace.Path, Mount: t.mount, Name: name, Version: v}
}

type keyData struct {
	metadata            transit.KeyMetadata
	keys                map[string]any
	derived, convergent bool
	receipt             engine.Response
}

func (t *TransitClient) readKey(ctx context.Context, name string, op engine.Operation) (*keyData, error) {
	if engine.ValidateSegment(name) != nil {
		return nil, invalid(string(op))
	}
	var out *keyData
	err := t.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + t.mount + "/keys/" + name}, t.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		bad := func() error { return engine.InvalidResponse(r, op) }
		n, ok := d["name"].(string)
		if !ok || n != name {
			return bad()
		}
		kind, ok := d["type"].(string)
		if !ok || kind == "" {
			return bad()
		}
		latest, ok := engine.Int(d["latest_version"])
		if !ok || latest <= 0 {
			return bad()
		}
		minEnc, ok := engine.Int(d["min_encryption_version"])
		if !ok || minEnc < 0 || minEnc > latest {
			return bad()
		}
		minDec, ok := engine.Int(d["min_decryption_version"])
		if !ok || minDec < 0 || minDec > latest {
			return bad()
		}
		m := transit.KeyMetadata{Name: n, Type: kind, LatestVersion: latest, MinEncryptionVersion: minEnc, MinDecryptionVersion: minDec, RequestID: r.RequestID}
		for _, v := range []struct {
			name string
			dst  *bool
		}{{"exportable", &m.Exportable}, {"deletion_allowed", &m.DeletionAllowed}, {"supports_signing", &m.SupportsSigning}, {"supports_encryption", &m.SupportsEncryption}} {
			b, yes := d[v.name].(bool)
			if !yes {
				return bad()
			}
			*v.dst = b
		}
		derived, ok := d["derived"].(bool)
		if !ok {
			return bad()
		}
		conv := false
		if v, exists := d["convergent_encryption"]; exists {
			conv, ok = v.(bool)
			if !ok {
				return bad()
			}
		}
		rawKeys, present := d["keys"]
		var keys map[string]any
		if !present && kind == "hmac" {
			// HMAC-only key metadata can omit creation timestamps for versions.
			keys = map[string]any{}
		} else {
			keys, ok = rawKeys.(map[string]any)
			if !ok || len(keys) == 0 && kind != "hmac" {
				return bad()
			}
		}
		for k := range keys {
			v, e := strconv.Atoi(k)
			if e != nil || v <= 0 || v > latest || strconv.Itoa(v) != k {
				return bad()
			}
			m.VersionNumbers = append(m.VersionNumbers, v)
		}
		sort.Ints(m.VersionNumbers)
		out = &keyData{metadata: m, keys: keys, derived: derived, convergent: conv, receipt: engine.Response{Status: r.Status, Attempts: r.Attempts, RequestID: r.RequestID}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
func (t *TransitClient) ReadKeyMetadata(ctx context.Context, name string) (*transit.KeyMetadata, error) {
	d, e := t.readKey(ctx, name, engine.TransitMetadata)
	if e != nil {
		return nil, e
	}
	m := d.metadata
	return &m, nil
}
func (t *TransitClient) public(ctx context.Context, name string, v int) (*transit.PublicKeyResult, crypto.PublicKey, error) {
	if v <= 0 {
		return nil, nil, invalid(string(engine.TransitPublicKey))
	}
	d, e := t.readKey(ctx, name, engine.TransitPublicKey)
	if e != nil {
		return nil, nil, e
	}
	entry, exists := d.keys[strconv.Itoa(v)]
	if !exists {
		return nil, nil, engine.SafeError(baoerr.CodeVersionUnavailable, engine.TransitPublicKey, 0, baoerr.EffectNone)
	}
	// No public-key caching or derived Ed25519 context guessing is permitted.
	if !d.metadata.SupportsSigning || d.derived {
		return nil, nil, invalid(string(engine.TransitPublicKey))
	}
	record, ok := entry.(map[string]any)
	if !ok {
		return nil, nil, engine.InvalidResponse(&d.receipt, engine.TransitPublicKey)
	}
	text, ok := record["public_key"].(string)
	if !ok {
		return nil, nil, engine.InvalidResponse(&d.receipt, engine.TransitPublicKey)
	}
	pub, der, pem, e := transitutil.Public(d.metadata.Type, text)
	if e != nil {
		return nil, nil, engine.InvalidResponse(&d.receipt, engine.TransitPublicKey)
	}
	return &transit.PublicKeyResult{Key: t.ref(name, v), KeyType: d.metadata.Type, SPKIDER: der, PEM: pem, RequestID: d.metadata.RequestID}, pub, nil
}
func (t *TransitClient) ReadPublicKey(ctx context.Context, name string, v int) (*transit.PublicKeyResult, error) {
	r, _, e := t.public(ctx, name, v)
	return r, e
}

// A compound operation has one budget, including its read-only preflight.
func (t *TransitClient) operationContext(ctx context.Context, op engine.Operation) (context.Context, context.CancelFunc, error) {
	if e := contextErr(ctx, op); e != nil {
		return nil, nil, e
	}
	c, cancel := context.WithTimeout(ctx, t.client.cfg.Timeouts.Request)
	return c, cancel, nil
}
