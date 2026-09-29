package bao

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/internal/testutil"
	"git.example.com/infra/openbao-sdk-go/pki"
	"git.example.com/infra/openbao-sdk-go/sensitive"
)

func TestLimitsRejectIntegerOverflow(t *testing.T) {
	c := testConfig()
	c.Limits.MaxResponseBytes = math.MaxInt64
	if _, e := New(c); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("response limit +1 can overflow")
	}
}
func TestPKIZeroTTLRejected(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	var n atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		jsonData(w, map[string]any{"certificate": string(f.CertificatePEM), "private_key": string(f.PrivateKeyPEM), "issuing_ca": string(f.CAPEM), "serial_number": "01:01", "expiration": f.Leaf.NotAfter.Unix()})
	}))
	p, _ := c.PKI("pki")
	if _, e := p.Issue(context.Background(), pki.IssueRequest{Role: "terminal", CommonName: "terminal.test"}); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("Issue zero TTL did not fail locally")
	}
	if _, e := p.SignCSR(context.Background(), pki.SignCSRRequest{Role: "terminal", CSRPEM: sensitive.NewBytes(f.CSRPEM)}); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("SignCSR zero TTL did not fail locally")
	}
	if n.Load() != 0 {
		t.Fatal("zero TTL reached service")
	}
}
func TestVoidErrorEnvelope(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"errors":["denied"]}`)) }))
	k, _ := c.KVv2("secret")
	if e := k.DeleteVersions(context.Background(), "item", []int{1}); !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("void write accepted error envelope as success")
	}
}

type sequenceTokenProvider struct {
	n                atomic.Int32
	entered, release chan struct{}
}

func (p *sequenceTokenProvider) Snapshot(ctx context.Context) (auth.TokenSnapshot, error) {
	if p.n.Add(1) == 1 {
		close(p.entered)
		select {
		case <-p.release:
			return auth.TokenSnapshot{}, errors.New("fixture startup rejected")
		case <-ctx.Done():
			return auth.TokenSnapshot{}, ctx.Err()
		}
	}
	return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("fixture-token"))}, nil
}
func TestStartResultBelongsToAttempt(t *testing.T) {
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)
	p := &sequenceTokenProvider{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := testConfig()
	cfg.Auth.TokenProvider = p
	c, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	first := make(chan [2]error, 1)
	go func() {
		e1 := c.Start(context.Background())
		e2 := c.Start(context.Background())
		first <- [2]error{e1, e2}
	}()
	<-p.entered
	waiter := make(chan error, 1)
	go func() { waiter <- c.Start(context.Background()) }()
	runtime.Gosched()
	close(p.release)
	r := <-first
	if r[0] == nil || r[1] != nil {
		t.Fatal("fixture did not fail once then succeed")
	}
	if e := <-waiter; !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) {
		t.Fatal("waiter observed a later attempt result")
	}
}
func TestCrossNamespaceConcurrency(t *testing.T) {
	var bad atomic.Int32
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		ns := r.Header.Get("X-Vault-Namespace")
		tok := r.Header.Get("X-Vault-Token")
		if (ns == "a" && tok != "fixture-a") || (ns == "b" && tok != "fixture-b") || (ns != "a" && ns != "b") {
			bad.Add(1)
		}
		jsonData(w, map[string]any{"data": map[string]any{"scope": ns}, "metadata": map[string]any{"version": 1, "created_time": "2026-01-01T00:00:00Z", "deletion_time": "", "destroyed": false}})
	})
	clients := make(map[string]*Client)
	for _, ns := range []string{"a", "b"} {
		cc := cfg
		cc.Namespace = NamespaceConfig{Mode: NamespaceNamed, Path: ns}
		tok, _ := auth.NewStaticToken(sensitive.NewBytes([]byte("fixture-" + ns)))
		cc.Auth.TokenProvider = tok
		clients[ns] = startedClient(t, cc)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		for _, ns := range []string{"a", "b"} {
			wg.Add(1)
			go func(ns string) {
				defer wg.Done()
				k, _ := clients[ns].KVv2("secret")
				r, e := k.ReadVersion(context.Background(), "same-path", 1)
				if e != nil {
					t.Error("scoped read failed")
					return
				}
				defer r.Data.Zero()
				var data struct {
					Scope string `json:"scope"`
				}
				if r.Data.Decode(&data) != nil || data.Scope != ns {
					t.Error("cross namespace response")
				}
			}(ns)
		}
	}
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatal("cross namespace identity")
	}
}
func TestAcceptedThenDisconnected(t *testing.T) {
	for _, op := range []engine.Operation{engine.PKIIssue, engine.KVCreate, engine.TransitSign} {
		t.Run(string(op), func(t *testing.T) {
			var writes atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				writes.Add(1)
				conn, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error("fixture hijack")
					return
				}
				conn.Close()
			}))
			e := c.execute(context.Background(), engine.Call{Operation: op, Path: "/v1/fixture/action", Payload: []byte(`{"value":"fixture"}`)}, "fixture", nil)
			if !baoerr.HasUnknownOutcome(e) || writes.Load() != 1 {
				t.Fatal("accepted write replayed or unknown lost")
			}
		})
	}
}
func TestCloseDuringWrite(t *testing.T) {
	entered := make(chan struct{})
	var calls atomic.Int32
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls.Add(1)
		close(entered)
		<-r.Context().Done()
	}))
	done := make(chan error, 1)
	go func() {
		done <- c.execute(context.Background(), engine.Call{Operation: engine.KVCreate, Path: "/v1/secret/data/item", Payload: []byte(`{"data":{}}`)}, "secret", nil)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	e := c.Close(ctx)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("shutdown budget not observed")
	}
	select {
	case e = <-done:
		if !baoerr.HasUnknownOutcome(e) || !errors.Is(e, context.Canceled) || calls.Load() != 1 {
			t.Fatal("canceled write effect wrong")
		}
	case <-time.After(time.Second):
		t.Fatal("write not canceled")
	}
}
func TestLoopbackTestConstructor(t *testing.T) {
	for _, addr := range []string{"http://example.com:8200", "http://192.168.1.1:8200", "http://localhost:8200"} {
		cfg := testConfig()
		cfg.Address = addr
		if _, e := newClient(cfg, true); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
			t.Fatal("test constructor escapes literal loopback")
		}
	}
	for _, addr := range []string{"http://127.0.0.1:8200", "http://[::1]:8200"} {
		cfg := testConfig()
		cfg.Address = addr
		c, e := newClient(cfg, true)
		if e != nil {
			t.Fatal("loopback exception unavailable")
		}
		c.Close(context.Background())
		if _, e = New(cfg); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
			t.Fatal("public HTTP exception")
		}
	}
}
