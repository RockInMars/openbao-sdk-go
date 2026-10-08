package bao

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

// Health is intentionally usable before Start. Its admitted request must still
// belong to Client.Close, not only to a caller or a future Start context.
func TestCloseCancelsHealthBeforeStart(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	caller, cancelCaller := context.WithCancel(context.Background())
	defer cancelCaller()
	done := make(chan error, 1)
	go func() { _, err := c.ClusterHealth(caller); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("health request never reached test server")
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelShutdown()
	if err = c.Close(shutdown); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Close did not report exhausted drain budget")
	}
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("Close did not cancel the admitted health request")
		}
	case <-time.After(time.Second):
		t.Fatal("pre-Start health request survived Close cancellation")
	}
	if err = c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartAfterLifetimeCancellationDoesNotReportReady(t *testing.T) {
	c, e := New(testConfig())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	if e = c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	cancel()
	if e = c.Start(context.Background()); !errors.Is(e, context.Canceled) {
		t.Fatal("Start reports success on canceled service lifetime")
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
