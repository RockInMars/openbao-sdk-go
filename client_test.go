package bao

import (
	"context"
	"encoding/pem"
	"errors"
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func serverConfig(t *testing.T, h http.HandlerFunc) Config {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	tok, e := auth.NewStaticToken(sensitive.NewBytes([]byte("fixture-token")))
	if e != nil {
		t.Fatal(e)
	}
	return Config{Address: srv.URL, ClusterAlias: "fixture", Namespace: NamespaceConfig{Mode: NamespaceRoot}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: tok}, TLS: TLSConfig{CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})}}
}
func startedClient(t *testing.T, cfg Config, opts ...Option) *Client {
	t.Helper()
	c, e := New(cfg, opts...)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if e = c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	return c
}
func simpleCall(c *Client, ctx context.Context) error {
	return c.execute(ctx, engine.Call{Operation: engine.KVReadVersion, Path: "/v1/kv/data/fixture"}, "kv", func(r *engine.Response) error { _, e := engine.Data(r, engine.KVReadVersion); return e })
}
func TestNewNoNetwork(t *testing.T) {
	var n atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.Write([]byte(`{"data":{}}`)) })
	c, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if n.Load() != 0 || c.State().Lifecycle != "CREATED" {
		t.Fatal("New has side effects")
	}
	if e = simpleCall(c, context.Background()); !baoerr.IsCode(e, baoerr.CodeNotReady) {
		t.Fatal("request before Start")
	}
	if e = c.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = c.Start(context.Background()); !baoerr.IsCode(e, baoerr.CodeClosed) {
		t.Fatal("closed client restarted")
	}
	if n.Load() != 0 {
		t.Fatal("close performed remote action")
	}
}
func TestConfigEnvironmentIgnored(t *testing.T) {
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "fixture-token" || r.Header.Get("X-Vault-Namespace") != "tenant-a" {
			t.Error("ambient identity override")
		}
		w.Write([]byte(`{"data":{}}`))
	})
	cfg.Namespace = NamespaceConfig{Mode: NamespaceNamed, Path: "tenant-a"}
	for _, key := range []string{"BAO_ADDR", "VAULT_ADDR", "BAO_AGENT_ADDR", "BAO_HTTP_PROXY", "HTTPS_PROXY", "HTTP_PROXY"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	t.Setenv("BAO_TOKEN", "ambient-fixture")
	t.Setenv("BAO_NAMESPACE", "ambient-scope")
	t.Setenv("BAO_SKIP_VERIFY", "true")
	t.Setenv("BAO_MAX_RETRIES", "999")
	c := startedClient(t, cfg)
	clear(cfg.TLS.CAPEM)
	if e := simpleCall(c, context.Background()); e != nil {
		t.Fatal(e)
	}
}

type countingProvider struct {
	n    atomic.Int32
	wait <-chan struct{}
}

func (p *countingProvider) Snapshot(ctx context.Context) (auth.TokenSnapshot, error) {
	p.n.Add(1)
	if p.wait != nil {
		select {
		case <-p.wait:
		case <-ctx.Done():
			return auth.TokenSnapshot{}, ctx.Err()
		}
	}
	return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("fixture-token"))}, nil
}
func TestStartSingleFlight(t *testing.T) {
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) { t.Error("Start must not query token ACL") })
	release := make(chan struct{})
	p := &countingProvider{wait: release}
	cfg.Auth.TokenProvider = p
	c, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := c.Start(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	close(release)
	wg.Wait()
	if p.n.Load() != 1 || !c.State().Ready {
		t.Fatal("concurrent Start duplicated")
	}
	_ = c.Close(context.Background())
}
func TestCloseDrainAndCancel(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			w.Write([]byte(`{"data":{}}`))
		case <-r.Context().Done():
		}
	})
	c := startedClient(t, cfg)
	done := make(chan error, 1)
	go func() { done <- simpleCall(c, context.Background()) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shutdown ignored budget")
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal("remaining request not canceled")
	}
	close(release)
	if !baoerr.IsCode(simpleCall(c, context.Background()), baoerr.CodeClosed) {
		t.Fatal("request after Close")
	}
	if e := c.Close(context.Background()); e != nil {
		t.Fatal("Close not idempotent")
	}
}
func TestTLSHostAndCARejected(t *testing.T) {
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid TLS reached handler") })
	cfg.TLS.ServerName = "wrong.invalid"
	c := startedClient(t, cfg)
	if e := simpleCall(c, context.Background()); e == nil {
		t.Fatal("hostname mismatch accepted")
	}
	cfg.TLS.ServerName = ""
	cfg.TLS.CAPEM = nil
	c2 := startedClient(t, cfg)
	if e := simpleCall(c2, context.Background()); e == nil {
		t.Fatal("untrusted CA accepted")
	}
}
