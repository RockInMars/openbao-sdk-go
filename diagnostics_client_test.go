package bao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"github.com/RockInMars/openbao-sdk-go/observe"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type fixtureSecretIDProvider struct{}

func (fixtureSecretIDProvider) Current(context.Context) (auth.SecretIDSnapshot, error) {
	return auth.SecretIDSnapshot{SecretID: sensitive.NewBytes([]byte("fixture-app-secret")), Generation: "fixture", Use: auth.ReusableSecretID}, nil
}

func TestManagedAppRoleObserverCountsLoginWithoutSecrets(t *testing.T) {
	var sent atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		if r.URL.Path != "/v1/auth/approle/login" || r.Header.Get("X-Vault-Token") != "" {
			t.Error("login request changed")
		}
		_, _ = w.Write([]byte(`{"auth":{"client_token":"fixture-session","lease_duration":60,"renewable":false}}`))
	})
	cfg.Auth = auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "approle", RoleID: sensitive.NewBytes([]byte("fixture-role")), SecretIDProvider: fixtureSecretIDProvider{}}}
	o := &recordingObserver{panicOn: true}
	c, err := New(cfg, WithObserver(o))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if sent.Load() != 1 || len(o.events) != 1 || o.events[0].Operation != "APPROLE_LOGIN" || o.events[0].Attempts != 1 {
		t.Fatalf("sent=%d events=%v", sent.Load(), o.events)
	}
	raw, _ := json.Marshal(o.events)
	if strings.Contains(string(raw), "fixture-role") || strings.Contains(string(raw), "fixture-app-secret") || strings.Contains(string(raw), "fixture-session") {
		t.Fatal("auth observer leaked a secret")
	}
}

func TestClusterHealthSemantics(t *testing.T) {
	for _, s := range []struct {
		code                         int
		initialized, sealed, standby bool
	}{{200, true, false, false}, {429, true, false, true}, {501, false, true, true}, {503, true, true, true}} {
		t.Run(fmt.Sprint(s.code), func(t *testing.T) {
			var n atomic.Int32
			cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				n.Add(1)
				if r.Header.Get("X-Vault-Token") != "" || r.Header.Get("X-Vault-Namespace") != "" {
					t.Error("health leaked identity")
				}
				if r.URL.Path != "/v1/sys/health" {
					t.Error("wrong health route")
				}
				w.WriteHeader(s.code)
				json.NewEncoder(w).Encode(map[string]any{"initialized": s.initialized, "sealed": s.sealed, "standby": s.standby, "version": "fixture-1", "cluster_id": "local-fixture"})
			})
			c, e := New(cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close(context.Background())
			h, e := c.ClusterHealth(context.Background())
			if e != nil || h.HTTPStatus != s.code || h.Initialized != s.initialized || h.Sealed != s.sealed || h.Standby != s.standby || n.Load() != 1 {
				t.Fatalf("health semantics: %v", e)
			}
			if c.State().Ready {
				t.Fatal("health implied auth readiness")
			}
		})
	}
}
func TestReadyScopedProbe(t *testing.T) {
	var denied atomic.Bool
	var calls atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/secret/data/probe" || r.URL.Query().Get("version") != "2" {
			t.Error("probe not exact scoped read")
		}
		if r.Header.Get("X-Vault-Namespace") != "space-a" || r.Header.Get("X-Vault-Token") == "" {
			t.Error("probe lost identity")
		}
		if denied.Load() {
			w.WriteHeader(403)
			w.Write([]byte(`{"errors":["permission denied"]}`))
			return
		}
		w.Write([]byte(`{"data":{"data":{"marker":"do-not-log"},"metadata":{"version":2,"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}}`))
	})
	cfg.Namespace = NamespaceConfig{Mode: NamespaceNamed, Path: "space-a"}
	c := startedClient(t, cfg)
	probe := diagnostics.ReadProbe{Mount: "secret", Path: "probe", Version: 2}
	r, e := c.CheckReady(context.Background(), probe)
	if e != nil || !r.Ready {
		t.Fatalf("probe: %v", e)
	}
	denied.Store(true)
	r, e = c.CheckReady(context.Background(), probe)
	if e != nil || r.Ready || r.ErrorCode != baoerr.CodePermissionDenied {
		t.Fatal("permission not represented in readiness")
	}
	n := calls.Load()
	probe.Version = 0
	if _, e = c.CheckReady(context.Background(), probe); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) || calls.Load() != n {
		t.Fatal("zero version probed")
	}
}

type recordingObserver struct {
	mu      sync.Mutex
	events  []observe.Event
	panicOn bool
}

func (o *recordingObserver) Observe(ctx context.Context, e observe.Event) {
	o.mu.Lock()
	o.events = append(o.events, e)
	o.mu.Unlock()
	if o.panicOn {
		panic("observer fixture panic")
	}
}
func TestObserverRedaction(t *testing.T) {
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"request_id":"ed7e019e-9d13-4c1a-9f01-01d5585a5b6a","data":{"version":1,"created_time":"2026-01-01T00:00:00Z"}}`))
	})
	o := &recordingObserver{panicOn: true}
	c, e := New(cfg, WithObserver(o))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	if e = c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	k, _ := c.KVv2("secret")
	d, _ := kv.NewDocument(map[string]any{"private_key": "sensitive-sentinel-body"})
	defer d.Zero()
	r, e := k.Create(context.Background(), "sensitive-sentinel-path", d)
	if e != nil || r.Ref.Version != 1 {
		t.Fatalf("observer changed write: %v", e)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.events) != 1 || o.events[0].Attempts != 1 || o.events[0].HTTPStatus != 200 {
		t.Fatal("missing outcome event")
	}
	b, _ := json.Marshal(o.events)
	for _, forbidden := range []string{"sensitive-sentinel-body", "sensitive-sentinel-path", "X-Vault-Token", "private_key"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatal("event leaks request contents")
		}
	}
}
