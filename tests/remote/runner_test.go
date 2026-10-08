package remotecheck

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// This simulator uses invented test data and binds only a local TLS listener.
type serviceFixture struct {
	mu                        sync.Mutex
	requests, writes, deletes int
	badHeaders                bool
	fault                     string
	versions                  map[int]map[string]string
	deleted                   bool
	extra                     func(http.ResponseWriter, *http.Request) bool
}

func (s *serviceFixture) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/v1/sys/health" {
		if r.Header.Get("X-Vault-Token") != "" || r.Header.Get("X-Vault-Namespace") != "" {
			s.badHeaders = true
		}
		if s.fault == "redirect" {
			w.Header().Set("Location", "https://not-authorized.invalid/v1/sys/health")
			w.WriteHeader(307)
			return
		}
		fmt.Fprint(w, `{"initialized":true,"sealed":false,"standby":false,"version":"2.4.0"}`)
		return
	}
	if r.Header.Get("X-Vault-Token") != "synthetic-token" || r.Header.Get("X-Vault-Namespace") != "sdk-test" {
		s.badHeaders = true
	}
	if s.fault == "forbidden" {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"errors":["synthetic-secret-response"]}`)
		return
	}
	if s.extra != nil && s.extra(w, r) {
		return
	}
	switch r.URL.Path {
	case "/v1/auth/token/lookup-self":
		if s.fault == "identity_forbidden" {
			w.WriteHeader(403)
			return
		}
		ttl, uses := 3600, 0
		if s.fault == "low_ttl" {
			ttl = 10
		}
		if s.fault == "low_uses" {
			uses = 7
		}
		fmt.Fprintf(w, `{"data":{"id":"synthetic-secret-response","accessor":"synthetic-accessor","ttl":%d,"num_uses":%d}}`, ttl, uses)
		return
	case "/v1/sys/capabilities-self":
		var input struct {
			Paths []string `json:"paths"`
		}
		json.NewDecoder(r.Body).Decode(&input)
		data := map[string]any{}
		for _, path := range input.Paths {
			data[path] = []string{"create", "update", "read", "delete"}
			if s.fault == "root_capability" {
				data[path] = []string{"root"}
			}
			if strings.Contains(path, "/encrypt/") && s.fault != "transit_create" {
				data[path] = []string{"update"}
			}
			if strings.Contains(path, "transit/keys/") && s.fault != "transit_key_admin" {
				data[path] = []string{"read"}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
		return
	case "/v1/kv/data/fixture/check":
		if s.fault == "malformed" {
			fmt.Fprint(w, `{"data":`)
			return
		}
		fmt.Fprint(w, `{"data":{"data":{"sdk_test_marker":"synthetic-marker"},"metadata":{"version":1,"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}}`)
		return
	}
	const path = "sdk-validation/local-fixture/kv"
	if r.URL.Path == "/v1/kv/metadata/"+path {
		if r.Method == "DELETE" {
			s.deletes++
			if s.fault == "lost_cleanup" {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			if s.fault == "cleanup" {
				w.WriteHeader(503)
				return
			}
			s.versions = map[int]map[string]string{}
			w.WriteHeader(204)
			return
		}
		if s.fault == "existing" {
			fmt.Fprint(w, `{"data":{"current_version":1}}`)
			return
		}
		if s.fault == "preflight_forbidden" {
			w.WriteHeader(403)
			return
		}
		if len(s.versions) == 0 {
			w.WriteHeader(404)
			return
		}
		versions := map[string]any{}
		for v := range s.versions {
			versions[strconv.Itoa(v)] = map[string]any{"created_time": "2026-01-01T00:00:00Z", "deletion_time": "", "destroyed": false}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"current_version": len(s.versions), "oldest_version": 1, "max_versions": 10, "cas_required": true, "delete_version_after": "0s", "versions": versions}})
		return
	}
	if r.URL.Path == "/v1/kv/data/"+path {
		if r.Method == "POST" || r.Method == "PUT" {
			s.writes++
			if s.fault == "stale_lost" && s.writes == 3 {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			if s.fault == "lost_write" {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			var input struct {
				Data    map[string]string `json:"data"`
				Options struct {
					CAS int `json:"cas"`
				} `json:"options"`
			}
			json.NewDecoder(r.Body).Decode(&input)
			if input.Options.CAS != len(s.versions) {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"errors":["check-and-set parameter did not match the current version"]}`)
				return
			}
			v := len(s.versions) + 1
			s.versions[v] = input.Data
			if s.fault == "lost_committed" {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			fmt.Fprintf(w, `{"data":{"version":%d,"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}`, v)
			return
		}
		v, _ := strconv.Atoi(r.URL.Query().Get("version"))
		data, exists := s.versions[v]
		if !exists || s.deleted && v == 1 {
			w.WriteHeader(404)
			return
		}
		if s.fault == "owner" {
			data = map[string]string{"sdk_test_owner": "someone-else"}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": data, "metadata": map[string]any{"version": v, "created_time": "2026-01-01T00:00:00Z", "deletion_time": "", "destroyed": false}}})
		return
	}
	if r.URL.Path == "/v1/kv/delete/"+path {
		s.deleted = true
		w.WriteHeader(204)
		return
	}
	if r.URL.Path == "/v1/kv/undelete/"+path {
		s.deleted = false
		w.WriteHeader(204)
		return
	}
	w.WriteHeader(404)
}

func localHarness(t *testing.T, mode, fault string) (*harness, *serviceFixture, *[]result) {
	t.Helper()
	s := &serviceFixture{fault: fault, versions: map[int]map[string]string{}}
	server := httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(server.Close)
	e := fixtureEnv()
	c, err := loadConfig(func(k string) string { return e[k] })
	if err != nil {
		t.Fatal(err)
	}
	// Only this private local constructor overrides the fixed remote address.
	c.address = server.URL
	c.mode = mode
	c.prefix = "sdk-validation"
	c.caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(c.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	records := []result{}
	h, err := newHarness(c, func(r result) error { records = append(records, r); return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.cancel)
	return h, s, &records
}

func TestReadOnlyUsesSDKAndSanitizedEvidence(t *testing.T) {
	h, s, records := localHarness(t, "readonly", "")
	r := h.run()
	if r.Status != "PASS" || r.Requests != 5 || s.requests != 5 || s.writes != 0 || s.badHeaders {
		t.Fatalf("unexpected result: status=%s count=%d actual=%d", r.Status, r.Requests, s.requests)
	}
	raw, _ := json.Marshal(records)
	for _, secret := range []string{"synthetic-token", "synthetic-secret-response", "synthetic-accessor", "synthetic-marker"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("sensitive data persisted")
		}
	}
}

func TestLocalFailureStopsBeforeWrites(t *testing.T) {
	for _, fault := range []string{"redirect", "forbidden", "malformed", "existing", "preflight_forbidden"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", fault)
			r := h.run()
			if r.Status == "PASS" || s.writes != 0 || s.deletes != 0 {
				t.Fatal("unsafe continuation")
			}
		})
	}
}

func TestTLSFailureAndBudgetExhaustion(t *testing.T) {
	h, s, _ := localHarness(t, "readonly", "")
	h.transport.TLSClientConfig.RootCAs = nil
	// The SDK's independent transport must also reject the untrusted server.
	c := h.c
	c.caFile = ""
	untrusted, err := newHarness(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := untrusted.run()
	if r.Status == "PASS" || s.requests != 0 {
		t.Fatal("untrusted TLS accepted")
	}
	h2, s2, _ := localHarness(t, "readonly", "")
	h2.b.limit = 1
	r = h2.run()
	if r.Status == "PASS" || s2.requests != 1 {
		t.Fatal("budget did not stop requests")
	}
}

func TestFullRequestBudgetsLeaveScenarioReserve(t *testing.T) {
	base, _, _ := localHarness(t, "isolated", "")
	for _, tc := range []struct {
		part             string
		used             int
		want             int
		minimumRemaining int
	}{
		{part: "core", used: 36, want: 43, minimumRemaining: 7},
		{part: "transit", used: 7, want: 32, minimumRemaining: 24},
	} {
		t.Run(tc.part, func(t *testing.T) {
			c := base.c
			c.full = true
			c.fullPart = tc.part
			c.manifestSHA = "synthetic-manifest-digest"
			c.expiresAt = time.Now().Add(10 * time.Minute)
			h, err := newHarness(c, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(h.cancel)
			t.Cleanup(h.transport.CloseIdleConnections)
			h.b.used = tc.used
			if h.b.limit != tc.want || h.b.limit-h.b.count() < tc.minimumRemaining {
				t.Fatalf("%s limit=%d remaining=%d", tc.part, h.b.limit, h.b.limit-h.b.count())
			}
		})
	}
}

func TestIsolatedKVAndCleanup(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "")
	h.c.softDelete = true
	r := h.run()
	if r.Status != "PASS" || s.writes != 3 || s.deletes != 1 || len(s.versions) != 0 || r.Requests != s.requests || r.Requests > 60 || r.Resources[0].State != "removed" {
		t.Fatalf("isolated status=%s cases=%+v", r.Status, r.Cases)
	}
}

func TestUnknownWriteIsNeverReplayed(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "lost_write")
	r := h.run()
	if r.Status == "PASS" || s.writes != 1 || s.deletes != 0 || r.Resources[0].State != "creation_pending" {
		t.Fatal("uncertain write replayed or ownership invented")
	}
	found := false
	for _, c := range r.Cases {
		if c.Name == "kv_create" && c.Unknown {
			found = true
		}
	}
	if !found {
		t.Fatal("UNKNOWN lost")
	}
}

func TestCleanupFailureAndOwnershipMismatchRemainPending(t *testing.T) {
	for _, fault := range []string{"cleanup", "owner"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", fault)
			r := h.run()
			if r.Status == "PASS" || r.Resources[0].State != "cleanup_pending" {
				t.Fatal("cleanup falsely passed")
			}
			if fault == "owner" && s.deletes != 0 {
				t.Fatal("foreign resource deleted")
			}
		})
	}
}

func TestJournalFailurePreventsMutation(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "")
	h.persist = func(r result) error {
		if len(r.Resources) > 0 {
			return errors.New("simulated persistence error")
		}
		return nil
	}
	r := h.run()
	if r.Status == "PASS" || s.writes != 0 {
		t.Fatal("mutation without recoverable journal")
	}
}

func TestJournalFailureAfterCreateKeepsOwnershipUnresolved(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "")
	h.persist = func(r result) error {
		for _, c := range r.Cases {
			if c.Name == "kv_create" {
				return errors.New("simulated checkpoint failure")
			}
		}
		return nil
	}
	r := h.run()
	if r.Status == "PASS" || s.writes != 1 || s.deletes != 0 || r.Resources[0].State != "creation_pending" {
		t.Fatal("lost checkpoint falsely claims no resource")
	}
}

func TestLostCleanupResponseIsUnknownAndNotReplayed(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "lost_cleanup")
	r := h.run()
	if r.Status == "PASS" || s.deletes != 1 || r.Resources[0].State != "cleanup_pending" {
		t.Fatal("cleanup replayed or hidden")
	}
	for _, c := range r.Cases {
		if c.Name == "kv_cleanup" && c.Unknown {
			return
		}
	}
	t.Fatal("cleanup UNKNOWN missing")
}

func TestInsufficientTokenLifetimeOrUsesPreventsMutation(t *testing.T) {
	for _, fault := range []string{"low_ttl", "low_uses"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", fault)
			r := h.run()
			if r.Status == "PASS" || s.writes != 0 {
				t.Fatal("insufficient token budget allowed mutation")
			}
		})
	}
}

func TestTransitCreateCapabilityIsRejectedBeforeMutation(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "transit_create")
	h.c.transitMount = "transit"
	h.c.transitKey = "test-key"
	h.c.transitType = "aes256-gcm96"
	r := h.run()
	if r.Status == "PASS" || s.writes != 0 {
		t.Fatal("implicit key creation privilege accepted")
	}
}

func TestRootCapabilityAndUnavailableIdentityPreventWrites(t *testing.T) {
	for _, fault := range []string{"root_capability", "identity_forbidden"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _ := localHarness(t, "isolated", fault)
			r := h.run()
			if r.Status == "PASS" || s.writes != 0 {
				t.Fatal("unbounded or unverified identity allowed writes")
			}
		})
	}
}

func TestStaleCASResponseLossKeepsUnknownAndPendingCleanup(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "stale_lost")
	r := h.run()
	if r.Status == "PASS" || s.writes != 3 || s.deletes != 0 || r.Resources[0].State != "cleanup_pending" {
		t.Fatal("stale CAS uncertainty mishandled")
	}
	for _, c := range r.Cases {
		if c.Name == "kv_stale_cas" && c.Unknown {
			return
		}
	}
	t.Fatal("stale CAS UNKNOWN missing")
}

func TestTransitKeyAdministrationPrivilegeIsRejected(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "transit_key_admin")
	h.c.transitMount = "transit"
	h.c.transitKey = "test-key"
	h.c.transitType = "ed25519"
	r := h.run()
	if r.Status == "PASS" || s.writes != 0 {
		t.Fatal("key administration privileges accepted")
	}
}

func TestUnknownCommittedCreateHasRecoverableOwnershipCommitment(t *testing.T) {
	h, s, _ := localHarness(t, "isolated", "lost_committed")
	r := h.run()
	if s.writes != 1 || s.deletes != 0 || r.Status == "PASS" {
		t.Fatal("uncertain commit mishandled")
	}
	raw, _ := json.Marshal(r.Resources[0])
	var record map[string]any
	json.Unmarshal(raw, &record)
	owner := s.versions[1]["sdk_test_owner"]
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(owner)))
	if owner == "" || record["ownership_sha256"] != digest || strings.Contains(string(raw), owner) {
		t.Fatal("ownership cannot be recovered without persisting KV content")
	}
}
