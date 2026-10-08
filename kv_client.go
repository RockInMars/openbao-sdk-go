package bao

import (
	"context"
	"encoding/json"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"math"
	"net/url"
	"strconv"
	"time"
)

// KVClient binds operations to one immutable KV v2 mount.
type KVClient struct {
	client *Client
	mount  string
}

// KVv2 binds an existing KV v2 mount within this client's fixed namespace.
// Binding is local; it neither creates a mount nor proves server permissions.
func (c *Client) KVv2(mount string) (*KVClient, error) {
	if c == nil || engine.ValidatePath(mount) != nil {
		return nil, invalid("KV_MOUNT")
	}
	return &KVClient{client: c, mount: mount}, nil
}
func (k *KVClient) ref(path string, v int) kv.Ref {
	return kv.Ref{ClusterAlias: k.client.cfg.ClusterAlias, Namespace: k.client.cfg.Namespace.Path, Mount: k.mount, Path: path, Version: v}
}

// Create uses CAS=0 to create a previously absent key. data remains caller-owned.
// On an unknown outcome, reconcile the key/version before considering a retry.
func (k *KVClient) Create(ctx context.Context, path string, data kv.Document) (*kv.WriteResult, error) {
	return k.write(ctx, path, 0, data, engine.KVCreate)
}

// CompareAndSwap writes only when the current version equals positive expected.
// It does not retry a conflicting or uncertain write; data remains caller-owned.
func (k *KVClient) CompareAndSwap(ctx context.Context, path string, expected int, data kv.Document) (*kv.WriteResult, error) {
	if expected <= 0 || expected == math.MaxInt {
		return nil, invalid(string(engine.KVCAS))
	}
	return k.write(ctx, path, expected, data, engine.KVCAS)
}
func (k *KVClient) write(ctx context.Context, path string, expected int, data kv.Document, op engine.Operation) (*kv.WriteResult, error) {
	if engine.ValidatePath(path) != nil {
		return nil, invalid(string(op))
	}
	raw := data.RevealJSON()
	defer clear(raw)
	if len(raw) == 0 || int64(len(raw)) > k.client.cfg.Limits.MaxRequestBytes {
		return nil, invalid(string(op))
	}
	payload, err := json.Marshal(struct {
		Options map[string]int  `json:"options"`
		Data    json.RawMessage `json:"data"`
	}{map[string]int{"cas": expected}, raw})
	if err != nil {
		return nil, invalid(string(op))
	}
	defer clear(payload)
	var result *kv.WriteResult
	err = k.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + k.mount + "/data/" + path, Payload: payload}, k.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		version, ok := engine.Int(d["version"])
		if !ok || version <= 0 || (expected > 0 && version != expected+1) {
			return engine.InvalidResponse(r, op)
		}
		created, ok := wireTime(d["created_time"])
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		result = &kv.WriteResult{Ref: k.ref(path, version), CreatedAt: created, RequestID: r.RequestID, Attempts: r.Attempts}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReadVersion reads an exact positive version. The caller must Zero result.Data.
func (k *KVClient) ReadVersion(ctx context.Context, path string, version int) (*kv.ReadResult, error) {
	if version <= 0 {
		return nil, invalid(string(engine.KVReadVersion))
	}
	return k.read(ctx, path, version, engine.KVReadVersion)
}

// ReadLatest requests the current latest version and returns its concrete Ref.
// The caller owns result.Data and must Zero it when finished.
func (k *KVClient) ReadLatest(ctx context.Context, path string) (*kv.ReadResult, error) {
	return k.read(ctx, path, 0, engine.KVReadLatest)
}

// ReadRef reads a version only if ref matches this client's cluster, namespace,
// and mount. It cannot change the client's authorization scope.
func (k *KVClient) ReadRef(ctx context.Context, ref kv.Ref) (*kv.ReadResult, error) {
	if ref.ClusterAlias != k.client.cfg.ClusterAlias || ref.Namespace != k.client.cfg.Namespace.Path || ref.Mount != k.mount {
		return nil, invalid(string(engine.KVReadVersion))
	}
	return k.ReadVersion(ctx, ref.Path, ref.Version)
}
func (k *KVClient) read(ctx context.Context, path string, version int, op engine.Operation) (*kv.ReadResult, error) {
	if engine.ValidatePath(path) != nil {
		return nil, invalid(string(op))
	}
	query := make(url.Values)
	if version > 0 {
		query.Set("version", strconv.Itoa(version))
	}
	var result *kv.ReadResult
	err := k.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + k.mount + "/data/" + path, Query: query}, k.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		meta, ok := d["metadata"].(map[string]any)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		current, ok := engine.Int(meta["version"])
		if !ok || current <= 0 || version > 0 && current != version {
			return engine.InvalidResponse(r, op)
		}
		v, e := versionMetadata(meta, current)
		if e != nil {
			return engine.InvalidResponse(r, op)
		}
		// deletion_time may be a future delete_version_after deadline, not
		// evidence that this version has already been soft-deleted.
		if v.Destroyed || (v.DeletedAt != nil && !v.DeletedAt.After(time.Now())) {
			return versionUnavailable(r, op)
		}
		values, ok := d["data"].(map[string]any)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		document, e := kv.NewDocument(values)
		if e != nil {
			return engine.InvalidResponse(r, op)
		}
		result = &kv.ReadResult{Ref: k.ref(path, current), Data: document, CreatedAt: v.CreatedAt, RequestID: r.RequestID, Attempts: r.Attempts}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
