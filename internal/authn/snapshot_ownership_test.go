package authn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type ownedTokenFunc struct{ tokenFunc }

func (p *ownedTokenFunc) SnapshotOwnedByConsumer(actual any) bool { return actual == p }

type ownedSecretFunc struct{ secretFunc }

func (p *ownedSecretFunc) SnapshotOwnedByConsumer(actual any) bool { return actual == p }

func TestOwnedTokenSnapshotsErasedOnEveryReturn(t *testing.T) {
	for _, outcome := range []string{"success", "error", "cancel", "invalid", "expired"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var snapshot sensitive.Bytes
			defer func() { snapshot.Zero() }()
			p := &ownedTokenFunc{func(context.Context) (auth.TokenSnapshot, error) {
				value := "fixture-token"
				if outcome == "invalid" {
					value = "bad\ncredential"
				}
				snapshot = sensitive.NewBytes([]byte(value))
				result := auth.TokenSnapshot{Token: snapshot}
				if outcome == "expired" {
					result.ValidUntil = time.Unix(1, 0)
				}
				if outcome == "cancel" {
					cancel()
				}
				if outcome == "error" {
					return result, errors.New("fixture provider failure")
				}
				return result, nil
			}}
			m := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: p}, nil, nil)
			defer m.Zero()
			token, err := m.Credential(ctx)
			if (err == nil) != (outcome == "success") || (err == nil && token != "fixture-token") {
				t.Fatal("credential outcome changed")
			}
			if snapshot.Len() != 0 {
				t.Fatal("owned TokenSnapshot retained after consumption")
			}
		})
	}
}

func TestOwnedSecretSnapshotsErasedOnEveryReturn(t *testing.T) {
	for _, outcome := range []string{"success", "error", "cancel", "invalid", "transport"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var snapshot sensitive.Bytes
			defer func() { snapshot.Zero() }()
			p := &ownedSecretFunc{func(context.Context) (auth.SecretIDSnapshot, error) {
				snapshot = sensitive.NewBytes([]byte("fixture-secret"))
				result := auth.SecretIDSnapshot{SecretID: snapshot, Generation: "fixture", Use: auth.ReusableSecretID}
				if outcome == "invalid" {
					result.Generation = ""
				}
				if outcome == "cancel" {
					cancel()
				}
				if outcome == "error" {
					return result, errors.New("fixture provider failure")
				}
				return result, nil
			}}
			role := sensitive.NewBytes([]byte("fixture-role"))
			defer role.Zero()
			m := New(auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "role", RoleID: role, SecretIDProvider: p}},
				authExecutor(func(context.Context, engine.Request) (*engine.Response, error) {
					if outcome == "transport" {
						return nil, &engine.TransportFailure{Cause: errors.New("fixture response lost"), Sent: true}
					}
					if outcome != "success" {
						t.Error("invalid snapshot reached transport")
					}
					return authReply(60, false), nil
				}), nil)
			defer m.Zero()
			err := m.Refresh(ctx)
			if (err == nil) != (outcome == "success") {
				t.Fatal("login outcome changed")
			}
			if outcome == "transport" && !baoerr.HasUnknownOutcome(err) {
				t.Fatal("unknown login outcome changed")
			}
			if snapshot.Len() != 0 || role.Len() == 0 {
				t.Fatal("wrong ownership cleared after login")
			}
		})
	}
}

type ownedTokenProvider interface {
	auth.TokenProvider
	SnapshotOwnedByConsumer(any) bool
}
type ownedSecretProvider interface {
	auth.SecretIDProvider
	SnapshotOwnedByConsumer(any) bool
}
type borrowingTokenWrapper struct {
	ownedTokenProvider
	shared sensitive.Bytes
}

func (p *borrowingTokenWrapper) Snapshot(context.Context) (auth.TokenSnapshot, error) {
	return auth.TokenSnapshot{Token: p.shared}, nil
}

type borrowingSecretWrapper struct {
	ownedSecretProvider
	shared sensitive.Bytes
}

func (p *borrowingSecretWrapper) Current(context.Context) (auth.SecretIDSnapshot, error) {
	return auth.SecretIDSnapshot{SecretID: p.shared, Generation: "fixture", Use: auth.ReusableSecretID}, nil
}

func TestProviderWrapperKeepsBorrowedSnapshots(t *testing.T) {
	shared := sensitive.NewBytes([]byte("fixture-shared"))
	defer shared.Zero()
	builtin, err := auth.NewStaticToken(shared)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := builtin.(ownedTokenProvider)
	if !ok {
		t.Fatal("built-in token ownership protocol missing")
	}
	for _, provider := range []auth.TokenProvider{
		tokenFunc(func(context.Context) (auth.TokenSnapshot, error) { return auth.TokenSnapshot{Token: shared}, nil }),
		&borrowingTokenWrapper{ownedTokenProvider: owner, shared: shared},
	} {
		m := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}, nil, nil)
		for i := 0; i < 2; i++ {
			if value, err := m.Credential(context.Background()); err != nil || value != "fixture-shared" {
				t.Fatal("borrowed token was invalidated")
			}
		}
		m.Zero()
		if shared.Len() == 0 {
			t.Fatal("borrowed token was erased")
		}
	}
	path := filepath.Join(t.TempDir(), "sid")
	if err := os.WriteFile(path, []byte("fixture-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	secret, err := auth.NewSecretIDFile(path, auth.ReusableSecretID)
	if err != nil {
		t.Fatal(err)
	}
	secretOwner, ok := secret.(ownedSecretProvider)
	if !ok {
		t.Fatal("built-in SecretID ownership protocol missing")
	}
	for _, provider := range []auth.SecretIDProvider{
		secretFunc(func(context.Context) (auth.SecretIDSnapshot, error) {
			return auth.SecretIDSnapshot{SecretID: shared, Generation: "fixture", Use: auth.ReusableSecretID}, nil
		}),
		&borrowingSecretWrapper{ownedSecretProvider: secretOwner, shared: shared},
	} {
		role := sensitive.NewBytes([]byte("fixture-role"))
		defer role.Zero()
		m := New(auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "role", RoleID: role, SecretIDProvider: provider}},
			authExecutor(func(context.Context, engine.Request) (*engine.Response, error) { return authReply(60, false), nil }), nil)
		for i := 0; i < 2; i++ {
			if err := m.login(context.Background()); err != nil {
				t.Fatal("borrowed SecretID was invalidated")
			}
		}
		m.Zero()
		if shared.Len() == 0 {
			t.Fatal("borrowed SecretID was erased")
		}
	}
}

type panicOwnershipProvider struct{ called bool }

func (p *panicOwnershipProvider) SnapshotOwnedByConsumer(any) bool {
	panic("fixture ownership secret")
}
func (p *panicOwnershipProvider) Snapshot(context.Context) (auth.TokenSnapshot, error) {
	p.called = true
	return auth.TokenSnapshot{}, nil
}
func (p *panicOwnershipProvider) Current(context.Context) (auth.SecretIDSnapshot, error) {
	p.called = true
	return auth.SecretIDSnapshot{}, nil
}

func TestProviderOwnershipPanicIsContainedBeforeSnapshot(t *testing.T) {
	p := &panicOwnershipProvider{}
	tokenManager := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: p}, nil, nil)
	_, err := tokenManager.Credential(context.Background())
	if !baoerr.IsCode(err, baoerr.CodeAuthenticationFailed) || strings.Contains(err.Error(), "fixture ownership secret") || p.called {
		t.Fatal("token ownership panic was not safely contained")
	}
	secretManager := New(auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{SecretIDProvider: p}}, authExecutor(nil), nil)
	err = secretManager.Refresh(context.Background())
	if !baoerr.IsCode(err, baoerr.CodeAuthenticationFailed) || strings.Contains(err.Error(), "fixture ownership secret") || p.called {
		t.Fatal("SecretID ownership panic was not safely contained")
	}
}

func TestOwnedTokenSnapshotsConcurrent(t *testing.T) {
	var mu sync.Mutex
	var snapshots []sensitive.Bytes
	p := &ownedTokenFunc{func(context.Context) (auth.TokenSnapshot, error) {
		snapshot := sensitive.NewBytes([]byte("fixture-concurrent"))
		mu.Lock()
		snapshots = append(snapshots, snapshot)
		mu.Unlock()
		return auth.TokenSnapshot{Token: snapshot}, nil
	}}
	m := New(auth.Config{Mode: auth.ExternalToken, TokenProvider: p}, nil, nil)
	defer m.Zero()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if value, err := m.Credential(context.Background()); err != nil || value != "fixture-concurrent" {
				t.Error("concurrent credential unavailable")
			}
		}()
	}
	wg.Wait()
	if len(snapshots) != 32 {
		t.Fatal("not all credential calls executed")
	}
	for _, snapshot := range snapshots {
		if snapshot.Len() != 0 {
			t.Error("concurrent snapshot retained")
		}
		snapshot.Zero()
	}
}
