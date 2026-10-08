package authn

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
)

func waitTimers(t *testing.T, c *fakeClock, number int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.timers)
		c.mu.Unlock()
		if n >= number {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("authentication loop did not create timer")
}
func TestRunStopsAndRestartsCanceledAttempt(t *testing.T) {
	clock := newFakeClock()
	var renews atomic.Int32
	signal := make(chan struct{}, 3)
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		if r.Path == "/v1/auth/token/renew-self" {
			renews.Add(1)
			signal <- struct{}{}
		}
		return authReply(60, true), nil
	}), clock)
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if e := m.Run(ctx); e != nil {
		t.Fatal(e)
	}
	waitTimers(t, clock, 1)
	cancel()
	if e := m.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	nextCtx, nextCancel := context.WithCancel(context.Background())
	defer nextCancel()
	if e := m.Run(nextCtx); e != nil {
		t.Fatal(e)
	}
	defer m.Stop(context.Background())
	waitTimers(t, clock, 2)
	clock.advance(40 * time.Second)
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("restarted loop never renewed")
	}
	if renews.Load() != 1 {
		t.Fatal("renewal loop duplicated")
	}
}
func TestAuthCapacityNotStarved(t *testing.T) {
	clock := newFakeClock()
	entered := make(chan struct{})
	release := make(chan struct{})
	var authCalls atomic.Int32
	ex := authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		if r.Method == "GET" {
			close(entered)
			select {
			case <-release:
				return &engine.Response{Status: 200, Body: []byte(`{"data":{}}`)}, nil
			case <-ctx.Done():
				return nil, &engine.TransportFailure{Cause: ctx.Err(), Sent: true}
			}
		}
		authCalls.Add(1)
		return authReply(60, true), nil
	})
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, ex, clock)
	defer m.Zero()
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := ex.Execute(context.Background(), engine.Call{Operation: engine.KVReadLatest, Path: "/v1/kv/data/item", Credential: m.Credential})
		done <- e
	}()
	<-entered
	clock.advance(40 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	e := m.Refresh(ctx)
	close(release)
	readErr := <-done
	if e != nil || readErr != nil || authCalls.Load() != 2 {
		t.Fatal("auth blocked behind business semaphore")
	}
}
func TestBusiness403DoesNotLoginOrReplay(t *testing.T) {
	var logins, writes atomic.Int32
	ex := authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		if r.Path == "/v1/auth/custom-role/login" {
			logins.Add(1)
			return authReply(60, true), nil
		}
		writes.Add(1)
		return &engine.Response{Status: 403, Body: []byte(`{"errors":["permission denied"]}`)}, nil
	})
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, ex, nil)
	defer m.Zero()
	if e := m.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, e := ex.Execute(context.Background(), engine.Call{Operation: engine.KVCreate, Path: "/v1/kv/data/item", Payload: []byte(`{"data":{}}`), Credential: m.Credential})
	if !baoerr.IsCode(e, baoerr.CodePermissionDenied) || logins.Load() != 1 || writes.Load() != 1 {
		t.Fatal("business 403 reauthenticated or replayed")
	}
}
func TestAuthStateAfterCanceledProvider(t *testing.T) {
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, authExecutor(func(ctx context.Context, r engine.Request) (*engine.Response, error) {
		return nil, &engine.TransportFailure{Cause: context.Canceled, Sent: true}
	}), nil)
	e := m.Refresh(context.Background())
	if !errors.Is(e, context.Canceled) || !baoerr.HasUnknownOutcome(e) || m.State().Ready {
		t.Fatal("unknown canceled authentication published")
	}
}
