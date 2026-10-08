package bao

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const fixtureTime = "2026-01-01T00:00:00Z"

func kvReadBody(version int, data string) string {
	return fmt.Sprintf(`{"data":{"data":%s,"metadata":{"version":%d,"created_time":"%s","deletion_time":"","destroyed":false}}}`, data, version, fixtureTime)
}
func kvWriteBody(version int) string {
	return fmt.Sprintf(`{"data":{"version":%d,"created_time":"%s","deletion_time":"","destroyed":false}}`, version, fixtureTime)
}
func TestKVCreateCASZero(t *testing.T) {
	var n atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Method != "POST" || r.URL.Path != "/v1/secret/data/a/b" {
			t.Error("KV creation route")
		}
		var b struct {
			Options struct {
				CAS *int `json:"cas"`
			} `json:"options"`
			Data json.RawMessage `json:"data"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil || b.Options.CAS == nil || *b.Options.CAS != 0 || !strings.Contains(string(b.Data), "9007199254740993") {
			t.Error("CAS or numeric data corrupted")
		}
		w.Write([]byte(kvWriteBody(1)))
	})
	c := startedClient(t, cfg)
	k, e := c.KVv2("secret")
	if e != nil {
		t.Fatal(e)
	}
	d, e := kv.ParseDocument([]byte(`{"number":9007199254740993}`))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Zero()
	result, e := k.Create(context.Background(), "a/b", d)
	if e != nil || result.Ref.Version != 1 || result.Attempts != 1 || result.Ref.ClusterAlias != "fixture" || n.Load() != 1 {
		t.Fatal("create result")
	}
}
func TestKVReadExactVersion(t *testing.T) {
	var n atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Query().Get("version") != "2" {
			t.Error("version omitted")
		}
		w.Write([]byte(kvReadBody(3, `{"value":"fixture"}`)))
	})
	c := startedClient(t, cfg)
	k, _ := c.KVv2("secret")
	if result, e := k.ReadVersion(context.Background(), "a", 2); e == nil || result != nil || !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || n.Load() != 1 {
		t.Fatal("wrong version accepted or retried")
	}
	if _, e := k.ReadVersion(context.Background(), "a", 0); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || n.Load() != 1 {
		t.Fatal("zero version sent")
	}
}
func TestKVReadRefBinding(t *testing.T) {
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.Write([]byte(kvReadBody(1, `{}`))) }))
	k, _ := c.KVv2("secret")
	good := kv.Ref{ClusterAlias: "fixture", Namespace: "", Mount: "secret", Path: "a", Version: 1}
	for i := 0; i < 3; i++ {
		bad := good
		if i == 0 {
			bad.ClusterAlias = "other"
		}
		if i == 1 {
			bad.Namespace = "other"
		}
		if i == 2 {
			bad.Mount = "other"
		}
		if _, e := k.ReadRef(context.Background(), bad); e == nil {
			t.Fatal("foreign ref accepted")
		}
	}
	if n.Load() != 0 {
		t.Fatal("ref rejection sent requests")
	}
	r, e := k.ReadRef(context.Background(), good)
	if e != nil || r.Ref != good {
		t.Fatal("bound ref failed")
	}
	r.Data.Zero()
}
func TestKVDeletedVersion(t *testing.T) {
	for _, status := range []int{200, 404} {
		c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"data":{"data":null,"metadata":{"version":1,"created_time":"2026-01-01T00:00:00Z","deletion_time":"2026-01-02T00:00:00Z","destroyed":false}}}`))
		}))
		k, _ := c.KVv2("secret")
		r, e := k.ReadVersion(context.Background(), "a", 1)
		if r != nil || !baoerr.IsCode(e, baoerr.CodeVersionUnavailable) {
			t.Fatal("deleted version accepted")
		}
	}
}
func TestKVCASConflict(t *testing.T) {
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(400)
		w.Write([]byte(`{"errors":["check-and-set parameter did not match the current version"]}`))
	}))
	k, _ := c.KVv2("secret")
	d, _ := kv.NewDocument(map[string]string{"v": "fixture"})
	defer d.Zero()
	_, e := k.CompareAndSwap(context.Background(), "a", 1, d)
	if !baoerr.IsCode(e, baoerr.CodeCASConflict) || baoerr.HasUnknownOutcome(e) || n.Load() != 1 {
		t.Fatal("CAS retried, overwritten, or misclassified")
	}
}
func TestKVCASConcurrentWinner(t *testing.T) {
	var mu sync.Mutex
	version := 1
	var arrivals atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		arrivals.Add(1)
		var b struct {
			Options struct {
				CAS int `json:"cas"`
			} `json:"options"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			t.Error("bad payload")
		}
		mu.Lock()
		defer mu.Unlock()
		if b.Options.CAS != version {
			w.WriteHeader(400)
			w.Write([]byte(`{"errors":["check-and-set parameter did not match the current version"]}`))
			return
		}
		version++
		w.Write([]byte(kvWriteBody(version)))
	}))
	k, _ := c.KVv2("secret")
	d, _ := kv.NewDocument(map[string]string{"v": "fixture"})
	defer d.Zero()
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := k.CompareAndSwap(context.Background(), "a", 1, d)
			if e == nil {
				success.Add(1)
			} else if !baoerr.IsCode(e, baoerr.CodeCASConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || arrivals.Load() != 20 {
		t.Fatal("CAS concurrency contract")
	}
}
func TestKVMetadataIsNotVersionHistory(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"current_version":2,"oldest_version":1,"max_versions":10,"cas_required":true,"delete_version_after":"0s","custom_metadata":{"label":"mutable"},"versions":{"2":{"created_time":"2026-01-02T00:00:00Z","deletion_time":"","destroyed":false},"1":{"created_time":"2026-01-01T00:00:00Z","deletion_time":"2026-01-03T00:00:00Z","destroyed":false}}}}`))
	}))
	k, _ := c.KVv2("secret")
	m, e := k.ReadMetadata(context.Background(), "a")
	if e != nil || len(m.Versions) != 2 || m.Versions[0].Version != 1 || m.Versions[0].DeletedAt == nil || m.CustomMetadata["label"] != "mutable" {
		t.Fatal("metadata decoding")
	}
}
func TestKVDeleteExactVersions(t *testing.T) {
	var routes []string
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		routes = append(routes, r.URL.Path)
		var b struct {
			Versions []int `json:"versions"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil || len(b.Versions) != 2 || b.Versions[0] != 1 || b.Versions[1] != 3 {
			t.Error("version list changed")
		}
		w.WriteHeader(204)
	}))
	k, _ := c.KVv2("secret")
	for _, bad := range [][]int{nil, {}, {0}, {-1}} {
		if e := k.DeleteVersions(context.Background(), "a", bad); e == nil {
			t.Fatal("invalid deletion accepted")
		}
	}
	if e := k.DeleteVersions(context.Background(), "a", []int{1, 3}); e != nil {
		t.Fatal(e)
	}
	if e := k.UndeleteVersions(context.Background(), "a", []int{1, 3}); e != nil {
		t.Fatal(e)
	}
	if len(routes) != 2 || routes[0] != "/v1/secret/delete/a" || routes[1] != "/v1/secret/undelete/a" {
		t.Fatal("destructive route substitution")
	}
}
func TestKVList404(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/secret/metadata/" || r.URL.RawQuery != "list=true" {
			t.Error("root list route")
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"errors":[]}`))
	}))
	k, _ := c.KVv2("secret")
	if r, e := k.List(context.Background(), ""); r != nil || !baoerr.IsCode(e, baoerr.CodeNotFoundOrHidden) {
		t.Fatal("404 converted to empty list")
	}
}

func TestKVListNestedFolderRoute(t *testing.T) {
	var actualMethod, actualPath, actualQuery string
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		actualMethod, actualPath, actualQuery = r.Method, r.URL.Path, r.URL.RawQuery
		if r.Method != http.MethodGet || r.URL.Path != "/v1/secret/metadata/sdk-validation/owned/" || r.URL.RawQuery != "list=true" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"keys":["seed"]}}`))
	}))
	k, _ := c.KVv2("secret")
	got, err := k.List(context.Background(), "sdk-validation/owned")
	if err != nil || got == nil || len(got.Entries) != 1 || got.Entries[0].Name != "seed" || got.Entries[0].IsFolder {
		t.Fatalf("nested folder list failed: method=%q path=%q query=%q err=%v", actualMethod, actualPath, actualQuery, err)
	}
}
