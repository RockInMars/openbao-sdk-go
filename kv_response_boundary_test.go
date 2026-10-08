package bao

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

func TestKVMetadataRejectsMalformedFields(t *testing.T) {
	base := `{"current_version":2,"oldest_version":1,"max_versions":10,"cas_required":true,"delete_version_after":"0s","custom_metadata":{"label":"ok"},"versions":{"1":{"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}}`
	cases := []struct {
		name, field string
		value       any
	}{
		{"negative", "current_version", -1}, {"fractional", "max_versions", 1.5},
		{"oldest newer", "oldest_version", 3}, {"cas type", "cas_required", "true"},
		{"ttl type", "delete_version_after", 1}, {"ttl invalid", "delete_version_after", "bad"},
		{"ttl negative", "delete_version_after", "-1s"}, {"custom type", "custom_metadata", []string{}},
		{"custom value", "custom_metadata", map[string]any{"key": 1}}, {"versions type", "versions", nil},
		{"version zero", "versions", map[string]any{"0": map[string]any{}}},
		{"version padded", "versions", map[string]any{"01": map[string]any{}}},
		{"version newer", "versions", map[string]any{"3": map[string]any{}}},
		{"version object", "versions", map[string]any{"1": nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(base), &data); err != nil {
				t.Fatal(err)
			}
			data[tc.field] = tc.value
			body, _ := json.Marshal(map[string]any{"data": data})
			var sent atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { sent.Add(1); w.Write(body) }))
			k, _ := c.KVv2("secret")
			result, err := k.ReadMetadata(context.Background(), "item")
			if result != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || sent.Load() != 1 {
				t.Fatalf("malformed metadata accepted or retried: %v", err)
			}
		})
	}
}

func TestKVVersionTimeAndDeletionShape(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
	}{
		{"created_time", nil}, {"created_time", ""}, {"created_time", "not-time"},
		{"destroyed", nil}, {"deletion_time", nil}, {"deletion_time", "0001-01-01T00:00:00Z"}, {"deletion_time", "invalid"},
	} {
		t.Run(tc.field+"-"+stringifyValue(tc.value), func(t *testing.T) {
			metadata := map[string]any{"version": 1, "created_time": fixtureTime, "deletion_time": "", "destroyed": false}
			metadata[tc.field] = tc.value
			body, _ := json.Marshal(map[string]any{"data": map[string]any{"data": map[string]any{}, "metadata": metadata}})
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
			k, _ := c.KVv2("secret")
			if got, err := k.ReadVersion(context.Background(), "item", 1); got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) {
				t.Fatalf("invalid version metadata accepted: %v", err)
			}
		})
	}
}

func stringifyValue(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestKVListValidOrderingAndInvalidEntries(t *testing.T) {
	for _, keys := range []any{nil, []any{1}, []string{""}, []string{"a", "a"}, []string{"../"}, []string{"a/b"}} {
		t.Run(stringifyValue(keys), func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"data": map[string]any{"keys": keys}})
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
			k, _ := c.KVv2("secret")
			if got, err := k.List(context.Background(), ""); got != nil || !baoerr.IsCode(err, baoerr.CodeInvalidResponse) {
				t.Fatalf("invalid list accepted: %v", err)
			}
		})
	}
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":{"keys":["z/","a/","a"]}}`)) }))
	k, _ := c.KVv2("secret")
	got, err := k.List(context.Background(), "")
	if err != nil || len(got.Entries) != 3 || got.Entries[0].Name != "a" || got.Entries[0].IsFolder || !got.Entries[1].IsFolder || got.Entries[2].Name != "z" {
		t.Fatalf("list order/folder lost: %v", err)
	}
}

func TestKVInvalidPathsAndDuplicateVersionsNeverSend(t *testing.T) {
	var sent atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { sent.Add(1) }))
	k, _ := c.KVv2("secret")
	if _, err := k.ReadMetadata(context.Background(), "../x"); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := k.List(context.Background(), "../x"); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
		t.Fatal(err)
	}
	for _, versions := range [][]int{{1, 1}, {0}, {-1}, nil} {
		if err := k.UndeleteVersions(context.Background(), "item", versions); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
			t.Fatal(err)
		}
	}
	if sent.Load() != 0 {
		t.Fatal("invalid request reached server")
	}
}
