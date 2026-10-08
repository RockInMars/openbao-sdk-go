package bao

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

func configWithOwnedInput(t *testing.T) Config {
	t.Helper()
	f := testutil.NewPKIFixture(t)
	c := testConfig()
	c.Auth = auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{
		Mount: "approle", RoleID: sensitive.NewBytes([]byte("fixture-role")), SecretIDProvider: fixtureSecretIDProvider{},
	}}
	c.TLS.ClientCertPEM = f.CertificatePEM
	c.TLS.ClientKeyPEM = sensitive.NewBytes(f.PrivateKeyPEM)
	t.Cleanup(c.Auth.AppRole.RoleID.Zero)
	t.Cleanup(c.TLS.ClientKeyPEM.Zero)
	return c
}

func requireSameSecret(t *testing.T, got sensitive.Bytes, want []byte) {
	t.Helper()
	raw := got.RevealCopy()
	defer clear(raw)
	if !bytes.Equal(raw, want) {
		t.Fatal("caller-owned secret changed")
	}
}

func TestClientConstructionFailureErasesOwnedSecrets(t *testing.T) {
	for _, name := range []string{"empty-option", "invalid-observer", "missing-ca", "malformed-ca"} {
		t.Run(name, func(t *testing.T) {
			cfg := configWithOwnedInput(t)
			var opts []Option
			switch name {
			case "empty-option":
				opts = []Option{{}}
			case "invalid-observer":
				opts = []Option{WithObserver(nil)}
			case "missing-ca":
				cfg.TLS.CAFile = filepath.Join(t.TempDir(), "missing.pem")
			case "malformed-ca":
				cfg.TLS.CAPEM = []byte("not a certificate")
			}
			originalKey := cfg.TLS.ClientKeyPEM.RevealCopy()
			defer clear(originalKey)
			owned, err := normalizeConfig(cfg, false)
			if err != nil {
				t.Fatal("test configuration did not reach client construction")
			}
			t.Cleanup(owned.Auth.AppRole.RoleID.Zero)
			t.Cleanup(owned.TLS.ClientKeyPEM.Zero)
			client, err := newClientFromConfig(owned, opts...)
			if client != nil {
				_ = client.Close(context.Background())
				t.Fatal("invalid construction returned a client")
			}
			if !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
				t.Fatal("expected the existing invalid-argument classification")
			}
			if owned.Auth.AppRole.RoleID.Len() != 0 || owned.TLS.ClientKeyPEM.Len() != 0 {
				t.Error("constructor failure retained SDK-owned secrets")
			}
			requireSameSecret(t, cfg.Auth.AppRole.RoleID, []byte("fixture-role"))
			requireSameSecret(t, cfg.TLS.ClientKeyPEM, originalKey)
		})
	}
}

func TestNormalizeConfigFailurePreservesCallerSecrets(t *testing.T) {
	for _, name := range []string{"conflicting-ca", "server-name", "proxy", "timeout", "limit", "retry"} {
		t.Run(name, func(t *testing.T) {
			cfg := configWithOwnedInput(t)
			originalKey := cfg.TLS.ClientKeyPEM.RevealCopy()
			defer clear(originalKey)
			switch name {
			case "conflicting-ca":
				cfg.TLS.CAFile, cfg.TLS.CAPEM = "unused", []byte("unused")
			case "server-name":
				cfg.TLS.ServerName = "bad\n"
			case "proxy":
				cfg.Network.ProxyURL = "ftp://invalid"
			case "timeout":
				cfg.Timeouts.Request = -1
			case "limit":
				cfg.Limits.MaxRequestBytes = -1
			case "retry":
				cfg.ReadRetry.MaxAttempts = 4
			}
			if _, err := normalizeConfig(cfg, false); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
				t.Fatal("invalid configuration accepted")
			}
			requireSameSecret(t, cfg.Auth.AppRole.RoleID, []byte("fixture-role"))
			requireSameSecret(t, cfg.TLS.ClientKeyPEM, originalKey)
		})
	}
}

func TestClientConstructionTransfersSecretsUntilClose(t *testing.T) {
	cfg := configWithOwnedInput(t)
	originalKey := cfg.TLS.ClientKeyPEM.RevealCopy()
	defer clear(originalKey)
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	owned := client.cfg
	if owned.Auth.AppRole.RoleID.Len() == 0 || owned.TLS.ClientKeyPEM.Len() == 0 {
		t.Fatal("successful construction erased its secrets")
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if owned.Auth.AppRole.RoleID.Len() != 0 || owned.TLS.ClientKeyPEM.Len() != 0 {
		t.Fatal("Close retained SDK-owned secrets")
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal("repeated Close failed")
	}
	requireSameSecret(t, cfg.Auth.AppRole.RoleID, []byte("fixture-role"))
	requireSameSecret(t, cfg.TLS.ClientKeyPEM, originalKey)
}
