package authn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type tokenFunc func(context.Context) (auth.TokenSnapshot, error)

func (f tokenFunc) Snapshot(c context.Context) (auth.TokenSnapshot, error) { return f(c) }

type secretFunc func(context.Context) (auth.SecretIDSnapshot, error)

func (f secretFunc) Current(c context.Context) (auth.SecretIDSnapshot, error) { return f(c) }

func TestExternalProviderFailureBoundaries(t *testing.T) {
	c := newFakeClock()
	cases := []tokenFunc{
		func(context.Context) (auth.TokenSnapshot, error) { panic("sensitive provider error") },
		func(context.Context) (auth.TokenSnapshot, error) {
			return auth.TokenSnapshot{}, errors.New("sensitive provider error")
		},
		func(context.Context) (auth.TokenSnapshot, error) {
			return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("bad\nheader"))}, nil
		},
		func(context.Context) (auth.TokenSnapshot, error) {
			return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte(strings.Repeat("a", 65537)))}, nil
		},
		func(context.Context) (auth.TokenSnapshot, error) {
			return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("fixture")), ValidUntil: c.Now()}, nil
		},
	}
	for _, p := range cases {
		m := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: p}, nil, c)
		_, e := m.Credential(context.Background())
		if !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) || m.State().Ready || strings.Contains(e.Error(), "sensitive provider error") {
			t.Fatal("bad provider was trusted or leaked")
		}
	}
	until := c.Now().Add(time.Hour)
	m := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: tokenFunc(func(context.Context) (auth.TokenSnapshot, error) {
		return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("fixture")), ValidUntil: until}, nil
	})}, nil, c)
	if e := m.Refresh(context.Background()); e != nil || !m.State().Ready {
		t.Fatal("valid external snapshot not accepted")
	}
	m.Zero()
	if m.State().AuthState != "CLOSED" {
		t.Fatal("closed state lost")
	}
}
func TestAuthenticationContextBoundaries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancelDeadline()
	for _, pair := range []struct {
		ctx  context.Context
		code string
	}{{nil, baoerr.CodeInvalidArgument}, {ctx, baoerr.CodeCanceled}, {deadline, baoerr.CodeDeadlineExceeded}} {
		m := New(auth.Config{Mode: auth.ManagedAppRole}, nil, nil)
		if _, e := m.Credential(pair.ctx); !baoerr.IsCode(e, pair.code) {
			t.Fatal("credential context not preserved")
		}
		if e := m.Refresh(pair.ctx); !baoerr.IsCode(e, pair.code) {
			t.Fatal("refresh context not preserved")
		}
		if e := m.Run(pair.ctx); !baoerr.IsCode(e, pair.code) {
			t.Fatal("run context not preserved")
		}
		if e := m.Stop(pair.ctx); !baoerr.IsCode(e, pair.code) {
			t.Fatal("stop context not preserved")
		}
	}
	m := New(auth.Config{Mode: auth.ManagedAppRole}, nil, nil)
	m.Zero()
	if e := m.Refresh(context.Background()); !baoerr.IsCode(e, baoerr.CodeClosed) {
		t.Fatal("refresh after zero")
	}
	if e := m.Run(context.Background()); !baoerr.IsCode(e, baoerr.CodeClosed) {
		t.Fatal("run after zero")
	}
	if e := m.Stop(context.Background()); e != nil {
		t.Fatal("stop without run not idempotent")
	}
}
func TestSecretProviderPanicAndInvalidGeneration(t *testing.T) {
	ex := authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
		t.Error("invalid provider reached network")
		return nil, nil
	})
	for _, p := range []secretFunc{
		func(context.Context) (auth.SecretIDSnapshot, error) { panic("provider secret") },
		func(context.Context) (auth.SecretIDSnapshot, error) {
			return auth.SecretIDSnapshot{SecretID: sensitive.NewBytes([]byte("valid")), Use: auth.SingleUseSecretID}, nil
		},
		func(context.Context) (auth.SecretIDSnapshot, error) {
			return auth.SecretIDSnapshot{SecretID: sensitive.NewBytes([]byte("valid")), Generation: "g", Use: "undefined"}, nil
		},
	} {
		m := New(auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "role", RoleID: sensitive.NewBytes([]byte("role")), SecretIDProvider: p}}, ex, nil)
		if e := m.Refresh(context.Background()); !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) {
			t.Fatal("invalid provider not rejected")
		}
	}
	m := New(auth.Config{Mode: auth.ManagedAppRole}, ex, nil)
	if e := m.Refresh(context.Background()); !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) {
		t.Fatal("missing AppRole not rejected")
	}
}
func TestExpiredReplyAndLatePublication(t *testing.T) {
	clock := newFakeClock()
	ex := authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
		clock.advance(2 * time.Second)
		return authReply(1, true), nil
	})
	m := managed(&sidProvider{g: "g", use: auth.ReusableSecretID}, ex, clock)
	if e := m.Refresh(context.Background()); !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || m.State().Ready {
		t.Fatal("response outlived lease but was published")
	}
	m2 := managed(&sidProvider{}, ex, clock)
	m2.Zero()
	if e := m2.publish(authReply(60, true), engine.AppRoleLogin, clock.Now(), 0); !baoerr.IsCode(e, baoerr.CodeClosed) || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("late login revived closed session")
	}
	m3 := managed(&sidProvider{}, ex, clock)
	if e := m3.publish(authReply(60, true), engine.TokenRenew, clock.Now(), 9); !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || m3.State().Ready {
		t.Fatal("stale renewal replaced session")
	}
}
