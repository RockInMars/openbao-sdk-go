package engine_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func executor(t *testing.T, h http.HandlerFunc) *engine.Executor {
	t.Helper()
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}))
	tr, e := engine.NewTransport(engine.TransportConfig{TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}, MaxResponseBytes: 1024, DialTimeout: time.Second, TLSHandshakeTimeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(tr.CloseIdleConnections)
	return engine.NewExecutor(testutil.NewHTTPSender(s.URL, "", engine.NewHTTPClient(tr), tr), engine.ExecutorConfig{RequestTimeout: time.Second, IssueTimeout: time.Second, LoginTimeout: 100 * time.Millisecond, RenewTimeout: 100 * time.Millisecond, Concurrency: 2, MaxRequestBytes: 1024, MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond})
}
func call(op engine.Operation) engine.Call {
	return engine.Call{Operation: op, Path: "/v1/kv/data/item", Payload: []byte(`{"data":{}}`), Credential: func(context.Context) (string, error) { return "fixture-token", nil }}
}
func TestSingleAttemptIssue(t *testing.T) {
	var n atomic.Int32
	e := executor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"errors":["unavailable"]}`))
	})
	_, err := e.Execute(context.Background(), call(engine.PKIIssue))
	if !baoerr.HasUnknownOutcome(err) || n.Load() != 1 {
		t.Fatal("write replayed or unknown outcome lost")
	}
}
func TestReadRetryBudget(t *testing.T) {
	var n atomic.Int32
	e := executor(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"errors":["busy"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	})
	c := call(engine.KVReadVersion)
	c.Payload = nil
	r, err := e.Execute(context.Background(), c)
	if err != nil || r.Attempts != 3 || n.Load() != 3 {
		t.Fatalf("read retry contract failed: error=%v arrivals=%d", err, n.Load())
	}
}
func TestNoWriteReplayHeader(t *testing.T) {
	var n atomic.Int32
	e := executor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("unexpected replay header")
		}
		w.Header().Set("Location", "https://untrusted.invalid")
		w.WriteHeader(307)
	})
	_, err := e.Execute(context.Background(), call(engine.KVCreate))
	if !baoerr.IsCode(err, baoerr.CodeRedirectBlocked) || n.Load() != 1 {
		t.Fatal("redirect followed")
	}
}
func TestResponseBoundAndSafeError(t *testing.T) {
	e := executor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		for i := 0; i < 300; i++ {
			_, _ = w.Write([]byte("private-fixture"))
		}
	})
	_, err := e.Execute(context.Background(), call(engine.KVCreate))
	if !baoerr.IsCode(err, baoerr.CodeResponseTooLarge) || !baoerr.HasUnknownOutcome(err) {
		t.Fatal("response not bounded")
	}
	if err == nil {
		t.Fatal("expected error")
	}
}
func TestCanceledBeforeAndAfterSend(t *testing.T) {
	var n atomic.Int32
	e := executor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Execute(ctx, call(engine.KVCreate))
	if !errors.Is(err, context.Canceled) || baoerr.HasUnknownOutcome(err) || n.Load() != 0 {
		t.Fatal("pre-dispatch cancellation")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = e.Execute(ctx, call(engine.KVCreate))
	if !errors.Is(err, context.DeadlineExceeded) || !baoerr.HasUnknownOutcome(err) || n.Load() != 1 {
		t.Fatalf("post-dispatch cancellation: error=%v unknown=%v arrivals=%d", err, baoerr.HasUnknownOutcome(err), n.Load())
	}
}
