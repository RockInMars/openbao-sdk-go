package bao

import (
	"context"
	"crypto/tls"
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"testing"
	"time"
)

type localTokenProvider struct{}

func (localTokenProvider) Snapshot(ctx context.Context) (auth.TokenSnapshot, error) {
	return auth.TokenSnapshot{Token: sensitive.NewBytes([]byte("local-test-token"))}, ctx.Err()
}
func testConfig() Config {
	p := localTokenProvider{}
	return Config{Address: "https://localhost:8200", ClusterAlias: "fixture", Namespace: NamespaceConfig{Mode: NamespaceRoot}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: p}}
}
func TestConfigDefaults(t *testing.T) {
	c, err := normalizeConfig(testConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	if c.Timeouts.Request != 10*time.Second || c.Timeouts.PKIIssue != 30*time.Second || c.Timeouts.Login != 10*time.Second || c.Timeouts.Renew != 5*time.Second || c.Timeouts.Dial != 5*time.Second || c.Timeouts.TLSHandshake != 5*time.Second {
		t.Fatal("timeout defaults")
	}
	if c.Limits.MaxConcurrentRequests != 32 || c.Limits.MaxRequestBytes != 1<<20 || c.Limits.MaxResponseBytes != 2<<20 || c.Limits.MaxResponseHeaderBytes != 64<<10 || c.TLS.MinVersion != tls.VersionTLS12 || c.ReadRetry.MaxAttempts != 3 {
		t.Fatal("security defaults")
	}
	c.Timeouts.Request = -1
	if _, e := normalizeConfig(c, false); e == nil {
		t.Fatal("negative timeout accepted")
	}
}
func TestNamespaceExplicit(t *testing.T) {
	for _, n := range []NamespaceConfig{{}, {Mode: NamespaceNamed}, {Mode: NamespaceRoot, Path: "x"}, {Mode: "other"}, {Mode: NamespaceNamed, Path: "a/../b"}} {
		c := testConfig()
		c.Namespace = n
		if _, e := normalizeConfig(c, false); e == nil {
			t.Fatal("ambiguous namespace accepted")
		}
	}
}
func TestTLSAndPathValidation(t *testing.T) {
	for _, addr := range []string{"http://localhost:8200", "https://user:pass@localhost", "https://localhost/v1", "https://localhost?x=1", "https://localhost#x", "ftp://localhost", "https://localhost:0"} {
		c := testConfig()
		c.Address = addr
		if _, e := normalizeConfig(c, false); e == nil {
			t.Fatal("unsafe address")
		}
	}
	c := testConfig()
	c.TLS.CAFile = "ca"
	c.TLS.CAPEM = []byte("ca")
	if _, e := normalizeConfig(c, false); e == nil {
		t.Fatal("conflicting CA source")
	}
	c = testConfig()
	c.TLS.MinVersion = tls.VersionTLS11
	if _, e := normalizeConfig(c, false); e == nil {
		t.Fatal("weak TLS accepted")
	}
}
