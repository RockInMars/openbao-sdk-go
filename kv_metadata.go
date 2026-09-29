package bao

import (
	"context"
	"encoding/json"
	"errors"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/kv"
	"sort"
	"strconv"
	"strings"
	"time"
)

func wireTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, e := time.Parse(time.RFC3339Nano, s)
	return t, e == nil && !t.IsZero()
}
func versionUnavailable(r *engine.Response, op engine.Operation) error {
	e := engine.SafeError(baoerr.CodeVersionUnavailable, op, r.Attempts, baoerr.EffectNone)
	e.HTTPStatus = r.Status
	e.RequestID = r.RequestID
	return e
}
func versionMetadata(m map[string]any, v int) (kv.VersionMetadata, error) {
	bad := errors.New("invalid version metadata")
	created, ok := wireTime(m["created_time"])
	if !ok {
		return kv.VersionMetadata{}, bad
	}
	destroyed, ok := m["destroyed"].(bool)
	if !ok {
		return kv.VersionMetadata{}, bad
	}
	deleted, ok := m["deletion_time"].(string)
	if !ok {
		return kv.VersionMetadata{}, bad
	}
	out := kv.VersionMetadata{Version: v, CreatedAt: created, Destroyed: destroyed}
	if deleted != "" {
		at, ok := wireTime(deleted)
		if !ok {
			return kv.VersionMetadata{}, bad
		}
		out.DeletedAt = &at
	}
	return out, nil
}
func (k *KVClient) ReadMetadata(ctx context.Context, path string) (*kv.Metadata, error) {
	op := engine.KVMetadata
	if engine.ValidatePath(path) != nil {
		return nil, invalid(string(op))
	}
	var result *kv.Metadata
	err := k.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + k.mount + "/metadata/" + path}, k.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		out := kv.Metadata{RequestID: r.RequestID}
		bad := func() error { return engine.InvalidResponse(r, op) }
		for _, field := range []struct {
			name string
			dst  *int
		}{{"current_version", &out.CurrentVersion}, {"oldest_version", &out.OldestVersion}, {"max_versions", &out.MaxVersions}} {
			n, ok := engine.Int(d[field.name])
			if !ok || n < 0 {
				return bad()
			}
			*field.dst = n
		}
		if out.OldestVersion > out.CurrentVersion {
			return bad()
		}
		cas, ok := d["cas_required"].(bool)
		if !ok {
			return bad()
		}
		out.CASRequired = cas
		ttl, ok := d["delete_version_after"].(string)
		if !ok {
			return bad()
		}
		dur, e := time.ParseDuration(ttl)
		if e != nil || dur < 0 {
			return bad()
		}
		out.DeleteVersionAfter = dur
		if data := d["custom_metadata"]; data != nil {
			m, ok := data.(map[string]any)
			if !ok {
				return bad()
			}
			out.CustomMetadata = make(map[string]string, len(m))
			for key, value := range m {
				s, ok := value.(string)
				if !ok {
					return bad()
				}
				out.CustomMetadata[key] = s
			}
		}
		versions, ok := d["versions"].(map[string]any)
		if !ok {
			return bad()
		}
		out.Versions = make([]kv.VersionMetadata, 0, len(versions))
		for key, raw := range versions {
			v, e := strconv.Atoi(key)
			if e != nil || v <= 0 || strconv.Itoa(v) != key || v > out.CurrentVersion {
				return bad()
			}
			m, ok := raw.(map[string]any)
			if !ok {
				return bad()
			}
			meta, e := versionMetadata(m, v)
			if e != nil {
				return bad()
			}
			out.Versions = append(out.Versions, meta)
		}
		sort.Slice(out.Versions, func(i, j int) bool { return out.Versions[i].Version < out.Versions[j].Version })
		result = &out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (k *KVClient) List(ctx context.Context, prefix string) (*kv.ListResult, error) {
	op := engine.KVList
	if prefix != "" && engine.ValidatePath(prefix) != nil {
		return nil, invalid(string(op))
	}
	var result *kv.ListResult
	err := k.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + k.mount + "/metadata/" + prefix}, k.mount, func(r *engine.Response) error {
		d, e := engine.Data(r, op)
		if e != nil {
			return e
		}
		keys, ok := d["keys"].([]any)
		if !ok {
			return engine.InvalidResponse(r, op)
		}
		out := &kv.ListResult{RequestID: r.RequestID, Entries: make([]kv.ListEntry, 0, len(keys))}
		seen := map[string]bool{}
		for _, raw := range keys {
			s, ok := raw.(string)
			if !ok || s == "" || seen[s] {
				return engine.InvalidResponse(r, op)
			}
			seen[s] = true
			folder := strings.HasSuffix(s, "/")
			name := strings.TrimSuffix(s, "/")
			if engine.ValidateSegment(name) != nil {
				return engine.InvalidResponse(r, op)
			}
			out.Entries = append(out.Entries, kv.ListEntry{Name: name, IsFolder: folder})
		}
		sort.Slice(out.Entries, func(i, j int) bool {
			a, b := out.Entries[i], out.Entries[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return !a.IsFolder && b.IsFolder
		})
		result = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (k *KVClient) DeleteVersions(ctx context.Context, path string, versions []int) error {
	return k.changeVersions(ctx, path, versions, false)
}
func (k *KVClient) UndeleteVersions(ctx context.Context, path string, versions []int) error {
	return k.changeVersions(ctx, path, versions, true)
}
func (k *KVClient) changeVersions(ctx context.Context, path string, versions []int, restore bool) error {
	op, route := engine.KVDelete, "delete"
	if restore {
		op, route = engine.KVUndelete, "undelete"
	}
	if engine.ValidatePath(path) != nil || len(versions) == 0 {
		return invalid(string(op))
	}
	seen := map[int]bool{}
	for _, v := range versions {
		if v <= 0 || seen[v] {
			return invalid(string(op))
		}
		seen[v] = true
	}
	payload, e := json.Marshal(map[string][]int{"versions": append([]int(nil), versions...)})
	if e != nil {
		return invalid(string(op))
	}
	return k.client.execute(ctx, engine.Call{Operation: op, Path: "/v1/" + k.mount + "/" + route + "/" + path, Payload: payload}, k.mount, nil)
}
