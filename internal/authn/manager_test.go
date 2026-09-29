package authn

import (
	"context"
	"encoding/json"
	"errors"
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type senderFunc func(context.Context, engine.Request) (*engine.Response, error)

func (f senderFunc) Send(c context.Context, r engine.Request) (*engine.Response, error) {
	return f(c, r)
}
func (f senderFunc) CloseIdleConnections() {}

type sidProvider struct {
	mu  sync.Mutex
	g   string
	use auth.SecretIDUse
}

func (p *sidProvider) Current(ctx context.Context) (auth.SecretIDSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return auth.SecretIDSnapshot{SecretID: sensitive.NewBytes([]byte("fixture-secret")), Generation: p.g, Use: p.use}, ctx.Err()
}
func (p *sidProvider) next(g string) { p.mu.Lock(); p.g = g; p.mu.Unlock() }
func authReply(ttl int, renew bool) *engine.Response {
	b, _ := json.Marshal(map[string]any{"auth": map[string]any{"client_token": "fixture-session", "lease_duration": ttl, "renewable": renew}})
	return &engine.Response{Status: 200, Header: make(http.Header), Body: b}
}
func authExecutor(f senderFunc) *engine.Executor {
	return engine.NewExecutor(f, engine.ExecutorConfig{RequestTimeout: time.Second, LoginTimeout: time.Second, RenewTimeout: time.Second, IssueTimeout: time.Second, MaxRequestBytes: 1 << 20, Concurrency: 1, MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, Namespace: "test-scope"})
}
func managed(s *sidProvider, ex *engine.Executor, clock Clock) *Manager {
	return New(auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "custom-role", RoleID: sensitive.NewBytes([]byte("fixture-role")), SecretIDProvider: s}}, ex, clock)
}
func TestAppRoleLoginContract(t *testing.T) {
	var count atomic.Int32
	ex := authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		count.Add(1)
		if r.Path != "/v1/auth/custom-role/login" || r.Namespace != "test-scope" || r.Token != "" {
			t.Error("login scope or token wrong")
		}
		var body map[string]string
		if json.Unmarshal(r.Body, &body) != nil || body["role_id"] != "fixture-role" || body["secret_id"] != "fixture-secret" {
			t.Error("login fields wrong")
		}
		return authReply(60, true), nil
	})
	m := managed(&sidProvider{g: "g1", use: auth.ReusableSecretID}, ex, nil)
	if err := m.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tok, err := m.Credential(context.Background()); err != nil || tok != "fixture-session" || count.Load() != 1 {
		t.Fatal("token not published")
	}
	if !m.State().Ready {
		t.Fatal("not ready")
	}
}
func TestAppRoleRejectEmptyOrInvalidLease(t *testing.T) {
	cases := []string{`{}`, `{"auth":{}}`, `{"auth":{"client_token":"fixture","lease_duration":0,"renewable":true}}`, `{"auth":{"client_token":"fixture","lease_duration":30}}`, `{"auth":{"client_token":"fixture","lease_duration":30,"renewable":true,"num_uses":1}}`}
	for _, body := range cases {
		m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
			return &engine.Response{Status: 200, Body: []byte(body)}, nil
		}), nil)
		err := m.Refresh(context.Background())
		if !baoerr.IsCode(err, baoerr.CodeInvalidResponse) || !baoerr.HasUnknownOutcome(err) || m.State().Ready {
			t.Fatal("invalid auth published")
		}
	}
}
func TestSingleUseSecretID(t *testing.T) {
	clock := newFakeClock()
	var n atomic.Int32
	p := &sidProvider{g: "one", use: auth.SingleUseSecretID}
	m := managed(p, authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
		if n.Add(1) == 1 {
			return nil, &engine.TransportFailure{Cause: errors.New("fixture connection lost"), Sent: true}
		}
		return authReply(60, false), nil
	}), clock)
	if err := m.Refresh(context.Background()); !baoerr.HasUnknownOutcome(err) {
		t.Fatal("unknown login outcome lost")
	}
	clock.advance(40 * time.Second)
	_ = m.Refresh(context.Background())
	if n.Load() != 1 {
		t.Fatal("single use retried")
	}
	p.next("two")
	clock.advance(40 * time.Second)
	if err := m.Refresh(context.Background()); err != nil || n.Load() != 2 {
		t.Fatal("new generation failed")
	}
}
func TestRenewUsesLatestTTL(t *testing.T) {
	clock := newFakeClock()
	var n atomic.Int32
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		if n.Add(1) == 1 {
			return authReply(60, true), nil
		}
		if r.Path != "/v1/auth/token/renew-self" || r.Token != "fixture-session" {
			t.Error("renew request wrong")
		}
		clock.advance(2 * time.Second)
		return authReply(10, true), nil
	}), clock)
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	first := *m.State().TokenExpiresAt
	clock.advance(40 * time.Second)
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	expiry := *m.State().TokenExpiresAt
	if !expiry.Before(first) || expiry.Sub(clock.Now()) != 8*time.Second {
		t.Fatal("renew ignored actual ttl or response delay")
	}
}
func TestRenewSingleFlight(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var n atomic.Int32
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
		if n.Add(1) == 1 {
			close(entered)
		}
		<-release
		return authReply(60, true), nil
	}), nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := m.Refresh(context.Background()); e != nil {
				t.Error("refresh failed")
			}
		}()
	}
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := m.Refresh(ctx); !errors.Is(e, context.Canceled) {
		t.Error("refresh wait not cancelable")
	}
	close(release)
	wg.Wait()
	if n.Load() != 1 {
		t.Fatal("refresh not combined")
	}
}
func TestExpiryAndShutdown(t *testing.T) {
	clock := newFakeClock()
	var n atomic.Int32
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
		if n.Add(1) == 1 {
			return authReply(10, true), nil
		}
		return nil, &engine.TransportFailure{Cause: errors.New("temporary"), Sent: true}
	}), clock)
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	clock.advance(7 * time.Second)
	_ = m.Refresh(context.Background())
	if s := m.State(); s.AuthState != "DEGRADED" || !s.Ready {
		t.Fatal("valid token not degraded")
	}
	clock.advance(4 * time.Second)
	if s := m.State(); s.Ready || s.AuthState != "AUTH_UNAVAILABLE" {
		t.Fatal("expired token considered ready")
	}
	if _, e := m.Credential(context.Background()); !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) {
		t.Fatal("expired token returned")
	}
	m.Zero()
	if _, e := m.Credential(context.Background()); !baoerr.IsCode(e, baoerr.CodeClosed) {
		t.Fatal("closed token returned")
	}
}

// A deterministic clock exercises expiry and renewal without sleeping out real TTLs.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}
type fakeTimer struct {
	ch      chan time.Time
	clock   *fakeClock
	when    time.Time
	stopped bool
}

func newFakeClock() *fakeClock      { return &fakeClock{now: time.Unix(1700000000, 0)} }
func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := &fakeTimer{ch: make(chan time.Time, 1), clock: c, when: c.now.Add(d)}
	c.timers = append(c.timers, f)
	return f
}
func (t *fakeTimer) C() <-chan time.Time { return t.ch }
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := !t.stopped
	t.stopped = true
	return was
}
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.stopped && !c.now.Before(t.when) {
			t.stopped = true
			t.ch <- c.now
		}
	}
}
