package testenv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// This is a fixture-builder contract test, not a real OpenBao integration test.
func TestBootstrapSchedulesDeletionWithoutRuntimeAdminPermission(t *testing.T) {
	var mu sync.Mutex
	scheduled := map[string]int{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ns := r.Header.Get("X-Vault-Namespace")
		path := strings.TrimPrefix(r.URL.Path, "/v1/")
		if r.Header.Get("X-Vault-Token") != "fixture-bootstrap-only" {
			t.Error("bootstrap identity missing")
		}
		if ns != "" && ns != "sdk-a" && ns != "sdk-b" {
			t.Error("namespace escaped fixture")
		}
		var args map[string]any
		if r.Method != "GET" && json.NewDecoder(r.Body).Decode(&args) != nil {
			t.Error("malformed bootstrap body")
		}
		switch {
		case path == "kv/metadata/fixture/scheduled":
			if r.Method != "POST" || args["delete_version_after"] != "1h" {
				t.Error("scheduled deletion configuration changed")
			}
			mu.Lock()
			scheduled[ns]++
			mu.Unlock()
		case path == "sys/policies/acl/provision":
			policy, ok := args["policy"].(string)
			if !ok || policy != runtimePolicy() {
				t.Error("runtime policy widened")
			}
		case path == "sys/policies/acl/control":
			if args["policy"] != controlPolicy() {
				t.Error("control policy widened")
			}
		}
		response := map[string]any{}
		switch {
		case path == "pki/root/generate/internal":
			response["data"] = map[string]string{"certificate": "public-fixture-issuer"}
		case strings.HasSuffix(path, "/role-id"):
			response["data"] = map[string]string{"role_id": "fixture-role"}
		case strings.HasSuffix(path, "/secret-id"):
			response["data"] = map[string]string{"secret_id": "fixture-secret-id"}
		case path == "auth/token/create":
			response["auth"] = map[string]string{"client_token": "fixture-runtime-token"}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error("fixture response could not be sent")
		}
	}))
	defer srv.Close()
	c := &Cluster{Address: srv.URL, http: srv.Client()}
	defer c.Close()
	if err := c.bootstrap(context.Background(), []byte("fixture-bootstrap-only")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(c.Spaces) != 2 || scheduled["sdk-a"] != 1 || scheduled["sdk-b"] != 1 {
		t.Fatal("scheduled-deletion fixture was not prepared in both namespaces")
	}
}
