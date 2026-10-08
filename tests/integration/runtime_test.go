//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
	"github.com/RockInMars/openbao-sdk-go/internal/testenv"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

type secretProvider struct {
	sid   sensitive.Bytes
	calls atomic.Int32
}

func (p *secretProvider) Current(ctx context.Context) (auth.SecretIDSnapshot, error) {
	p.calls.Add(1)
	return auth.SecretIDSnapshot{SecretID: p.sid, Generation: "isolated-fixture-generation", Use: auth.ReusableSecretID}, ctx.Err()
}
func newClient(t *testing.T, ctx context.Context, c *testenv.Cluster, s testenv.Space, i testenv.Identity, managed bool) (*bao.Client, *secretProvider) {
	t.Helper()
	cfg := bao.Config{Address: c.Address, ClusterAlias: "isolated", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceNamed, Path: s.Namespace}, TLS: bao.TLSConfig{CAPEM: c.CAPEM}}
	var sp *secretProvider
	if managed {
		sp = &secretProvider{sid: sensitive.NewBytes(i.SecretID)}
		cfg.Auth = auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: "sdk-role", RoleID: sensitive.NewBytes(i.RoleID), SecretIDProvider: sp}}
	} else {
		token := sensitive.NewBytes(i.Token)
		provider, e := auth.NewStaticToken(token)
		token.Zero()
		if e != nil {
			t.Fatal(e)
		}
		cfg.Auth = auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}
	}
	x, e := bao.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.Start(ctx); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := x.Close(closeCtx); e != nil {
			t.Error(e)
		}
		if sp != nil {
			sp.sid.Zero()
		}
	})
	return x, sp
}
func TestIntegrationRuntime(t *testing.T) {
	options, e := testenv.OptionsFromEnv()
	if e != nil {
		t.Fatal(e)
	} // FAIL, never Skip.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cluster, e := testenv.Launch(ctx, options)
	if e != nil {
		t.Fatal(e)
	}
	defer cluster.Close()
	t.Log("EVIDENCE_CLASS=REAL_OPENBAO; temporary TLS cluster; runtime assertions use restricted identities")
	if len(cluster.Spaces) != 2 {
		t.Fatal("fixture namespace count")
	}
	t.Run("TestIntegrationKV", func(t *testing.T) {
		c, _ := newClient(t, ctx, cluster, cluster.Spaces[0], cluster.Spaces[0].Provision, false)
		k, e := c.KVv2("kv")
		if e != nil {
			t.Fatal(e)
		}
		// This lifecycle fixture uses a number the pinned backend preserves.
		// TestDocumentExactNumber independently covers the SDK's full-precision JSON contract.
		doc, e := kv.ParseDocument([]byte(`{"integer":9007199254740991,"operation_id":"owned"}`))
		if e != nil {
			t.Fatal(e)
		}
		defer doc.Zero()
		w, e := k.Create(ctx, "fixture/kv", doc)
		if e != nil {
			t.Fatal(e)
		}
		if w.Ref.Version != 1 {
			t.Fatal("first version")
		}
		var wins atomic.Int32
		var wg sync.WaitGroup
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := k.CompareAndSwap(ctx, "fixture/kv", 1, doc)
				if e == nil {
					wins.Add(1)
				} else if !baoerr.IsCode(e, baoerr.CodeCASConflict) {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatal("CAS winner count")
		}
		r, e := k.ReadRef(ctx, w.Ref)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Data.Zero()
		var data map[string]any
		if e = r.Data.Decode(&data); e != nil {
			t.Fatal(e)
		}
		if data["integer"] != json.Number("9007199254740991") {
			t.Fatal("integer precision")
		}
		if e = k.DeleteVersions(ctx, "fixture/kv", []int{1}); e != nil {
			t.Fatal(e)
		}
		if _, e = k.ReadVersion(ctx, "fixture/kv", 1); !baoerr.IsCode(e, baoerr.CodeVersionUnavailable) {
			t.Fatalf("deleted version classification: %v", e)
		}
		if e = k.UndeleteVersions(ctx, "fixture/kv", []int{1}); e != nil {
			t.Fatal(e)
		}
		restored, e := k.ReadVersion(ctx, "fixture/kv", 1)
		if e != nil {
			t.Fatal(e)
		}
		restored.Data.Zero()
		latest, e := k.ReadLatest(ctx, "fixture/kv")
		if e != nil {
			t.Fatal(e)
		}
		defer latest.Data.Zero()
		if latest.Ref.Version != 2 {
			t.Fatal("latest explicit version")
		}
		meta, e := k.ReadMetadata(ctx, "fixture/kv")
		if e != nil || meta.CurrentVersion != 2 {
			t.Fatal("metadata", e)
		}
		entries, e := k.List(ctx, "fixture")
		if e != nil || len(entries.Entries) == 0 {
			t.Fatal("list", e)
		}
		wrong := w.Ref
		wrong.Namespace = cluster.Spaces[1].Namespace
		if _, e = k.ReadRef(ctx, wrong); !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
			t.Fatal("cross namespace reference accepted")
		}
		if _, e = k.Create(ctx, "outside-policy/path", doc); !baoerr.IsCode(e, baoerr.CodePermissionDenied) {
			t.Fatal("policy boundary", e)
		}
		ready, e := c.CheckReady(ctx, diagnostics.ReadProbe{Mount: "kv", Path: "fixture/kv", Version: 1})
		if e != nil || !ready.Ready {
			t.Fatal("readiness", e)
		}
	})
	t.Run("TestIntegrationKVScheduledDeletion", func(t *testing.T) {
		c, _ := newClient(t, ctx, cluster, cluster.Spaces[0], cluster.Spaces[0].Provision, false)
		k, err := c.KVv2("kv")
		if err != nil {
			t.Fatal(err)
		}
		doc, err := kv.NewDocument(map[string]string{"operation_id": "scheduled-integration"})
		if err != nil {
			t.Fatal(err)
		}
		defer doc.Zero()
		written, err := k.Create(ctx, "fixture/scheduled", doc)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := k.ReadMetadata(ctx, "fixture/scheduled")
		if err != nil {
			t.Fatal(err)
		}
		if len(meta.Versions) != 1 || meta.Versions[0].DeletedAt == nil || !meta.Versions[0].DeletedAt.After(time.Now()) {
			t.Fatal("server did not return a future scheduled deletion")
		}
		read, err := k.ReadRef(ctx, written.Ref)
		if err != nil {
			t.Fatal(err)
		}
		defer read.Data.Zero()
		if read.Ref != written.Ref {
			t.Fatal("scheduled read changed exact reference")
		}
		if err = k.DeleteVersions(ctx, written.Ref.Path, []int{written.Ref.Version}); err != nil {
			t.Fatal(err)
		}
		if _, err = k.ReadRef(ctx, written.Ref); !baoerr.IsCode(err, baoerr.CodeVersionUnavailable) {
			t.Fatal("explicit deletion did not take precedence over future schedule")
		}
	})
	t.Run("TestIntegrationPKI", func(t *testing.T) {
		s := cluster.Spaces[0]
		c, _ := newClient(t, ctx, cluster, s, s.Provision, false)
		p, e := c.PKI("pki")
		if e != nil {
			t.Fatal(e)
		}
		issued, e := p.Issue(ctx, pki.IssueRequest{Role: "device", CommonName: "terminal.test", TTL: 5 * time.Minute})
		if e != nil {
			t.Fatal(e)
		}
		defer issued.PrivateKey.Zero()
		policy := pki.VerifyPolicy{TrustedRootsPEM: [][]byte{s.IssuerCA}, RequiredEKUs: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, ExpectedCN: "terminal.test", ExpectedDNS: []string{"terminal.test"}, MinRemainingTTL: time.Minute}
		if e = pki.ValidateBundle(*issued, policy); e != nil {
			t.Fatal(e)
		}
		keyCopy := issued.PrivateKey.RevealCopy()
		defer clear(keyCopy)
		doc, e := kv.NewDocument(map[string]any{"certificate": string(issued.Certificate.CertificatePEM), "private_key": string(keyCopy), "operation_id": "pki-integration"})
		if e != nil {
			t.Fatal(e)
		}
		defer doc.Zero()
		k, e := c.KVv2("kv")
		if e != nil {
			t.Fatal(e)
		}
		ref, e := k.Create(ctx, "fixture/terminal-private", doc)
		if e != nil {
			t.Fatal(e)
		}
		read, e := k.ReadRef(ctx, ref.Ref)
		if e != nil {
			t.Fatal(e)
		}
		read.Data.Zero()
		record, e := p.ReadCertificate(ctx, issued.Certificate.SerialNumber)
		if e != nil || !bytes.Equal(record.CertificatePEM, issued.Certificate.CertificatePEM) {
			t.Fatal("read certificate", e)
		}
		chain, e := p.ReadIssuerChain(ctx)
		if e != nil || len(chain.CertificatesPEM) == 0 {
			t.Fatal("current chain", e)
		}
		priv, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		csr, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "terminal.test"}, DNSNames: []string{"terminal.test"}}, priv)
		if e != nil {
			t.Fatal(e)
		}
		request := sensitive.NewBytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))
		defer request.Zero()
		signed, e := p.SignCSR(ctx, pki.SignCSRRequest{Role: "device", CSRPEM: request, TTL: 5 * time.Minute})
		if e != nil || len(signed.Certificate.CertificatePEM) == 0 {
			t.Fatal("CSR sign", e)
		}
		if _, e = p.Issue(ctx, pki.IssueRequest{Role: "unauthorized", CommonName: "terminal.test", TTL: time.Minute}); !baoerr.IsCode(e, baoerr.CodePermissionDenied) {
			t.Fatal("unauthorized role", e)
		}
		if _, e = p.Revoke(ctx, issued.Certificate.SerialNumber); !baoerr.IsCode(e, baoerr.CodePermissionDenied) {
			t.Fatal("runtime revoked certificate without maintenance permission", e)
		}
		maintenance, _ := newClient(t, ctx, cluster, s, s.Maintenance, false)
		revoker, e := maintenance.PKI("pki")
		if e != nil {
			t.Fatal(e)
		}
		rv, e := revoker.Revoke(ctx, issued.Certificate.SerialNumber)
		if e != nil || rv.RevokedAt.IsZero() {
			t.Fatal("revocation receipt", e)
		}
	})
	t.Run("TestIntegrationTransit", func(t *testing.T) {
		s := cluster.Spaces[0]
		c, _ := newClient(t, ctx, cluster, s, s.Control, false)
		tr, e := c.Transit("transit")
		if e != nil {
			t.Fatal(e)
		}
		msg := sensitive.NewBytes([]byte("isolated runtime message"))
		defer msg.Zero()
		h := sha256.Sum256(msg.RevealCopy())
		digest := sensitive.NewBytes(h[:])
		defer digest.Zero()
		for _, item := range []struct {
			name    string
			profile transit.Profile
		}{{"ecdsa", transit.ECDSAP256SHA256ASN1}, {"rsa", transit.RSAPSSSHA256}, {"ed25519", transit.Ed25519Message}} {
			for _, v := range []int{1, 2} {
				sign, e := tr.Sign(ctx, transit.SignRequest{KeyName: item.name, KeyVersion: v, Profile: item.profile, Message: msg})
				if e != nil {
					t.Fatal(e)
				}
				valid, e := tr.Verify(ctx, transit.VerifyRequest{KeyName: item.name, ExpectedVersion: v, Profile: item.profile, Message: msg, Signature: sign.Signature})
				if e != nil || !valid.Valid {
					t.Fatal("verify", e)
				}
				pub, e := tr.ReadPublicKey(ctx, item.name, v)
				if e != nil || pub.Key.Version != v {
					t.Fatal("historical key", e)
				}
				if item.profile != transit.Ed25519Message {
					d, e := tr.SignDigest(ctx, transit.SignDigestRequest{KeyName: item.name, KeyVersion: v, Profile: item.profile, Digest: digest})
					if e != nil {
						t.Fatal(e)
					}
					verify, e := tr.VerifyDigest(ctx, transit.VerifyDigestRequest{KeyName: item.name, ExpectedVersion: v, Profile: item.profile, Digest: digest, Signature: d.Signature})
					if e != nil || !verify.Valid {
						t.Fatal("digest verify", e)
					}
				}
			}
		}
		for _, derived := range []bool{false, true} {
			key := "cipher"
			contextBytes := sensitive.Bytes{}
			if derived {
				key = "derived"
				contextBytes = sensitive.NewBytes([]byte("owned-key-domain"))
				defer contextBytes.Zero()
			}
			enc, e := tr.Encrypt(ctx, transit.EncryptRequest{KeyName: key, KeyVersion: 1, Plaintext: msg, Context: contextBytes})
			if e != nil {
				t.Fatal(e)
			}
			re, e := tr.Rewrap(ctx, transit.RewrapRequest{KeyName: key, TargetVersion: 2, Ciphertext: enc.Ciphertext, Context: contextBytes})
			if e != nil || re.Ciphertext.Version != 2 {
				t.Fatal("rewrap", e)
			}
			plain, e := tr.Decrypt(ctx, transit.DecryptRequest{KeyName: key, Ciphertext: re.Ciphertext, Context: contextBytes})
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(plain.Plaintext.RevealCopy(), msg.RevealCopy()) {
				t.Fatal("cipher round trip")
			}
			plain.Plaintext.Zero()
		}
		// Pinned HMAC metadata has no version history; only latest is available.
		if old, e := tr.HMAC(ctx, transit.HMACRequest{KeyName: "mac", KeyVersion: 1, Message: msg}); old != nil || !baoerr.IsCode(e, baoerr.CodeVersionUnavailable) {
			t.Fatal("unlisted historical HMAC version accepted", e)
		}
		mac, e := tr.HMAC(ctx, transit.HMACRequest{KeyName: "mac", KeyVersion: 2, Message: msg})
		if e != nil {
			t.Fatal(e)
		}
		if mac.Version != 2 {
			t.Fatal("HMAC version differs from requested latest")
		}
		v, e := tr.HMACVerify(ctx, transit.HMACVerifyRequest{KeyName: "mac", ExpectedVersion: 2, Message: msg, WrappedHMAC: mac.Wrapped})
		if e != nil || !v.Valid {
			t.Fatal("HMAC", e)
		}
		altered := sensitive.NewBytes([]byte("altered message"))
		defer altered.Zero()
		v, e = tr.HMACVerify(ctx, transit.HMACVerifyRequest{KeyName: "mac", ExpectedVersion: 2, Message: altered, WrappedHMAC: mac.Wrapped})
		if e != nil || v.Valid {
			t.Fatal("invalid HMAC", e)
		}
		if _, e = tr.Encrypt(ctx, transit.EncryptRequest{KeyName: "unknown", KeyVersion: 1, Plaintext: msg}); e == nil {
			t.Fatal("unknown key created")
		}
		k, e := c.KVv2("kv")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = k.ReadVersion(ctx, "fixture/terminal-private", 1); !baoerr.IsCode(e, baoerr.CodePermissionDenied) {
			t.Fatal("control identity read terminal private key", e)
		}
	})
	t.Run("TestIntegrationAuthNamespace", func(t *testing.T) {
		clients := make([]*bao.Client, 0, 2)
		providers := make([]*secretProvider, 0, 2)
		for _, s := range cluster.Spaces {
			c, p := newClient(t, ctx, cluster, s, s.Provision, true)
			clients = append(clients, c)
			providers = append(providers, p)
			k, e := c.KVv2("kv")
			if e != nil {
				t.Fatal(e)
			}
			doc, e := kv.NewDocument(map[string]any{"owner": s.Namespace})
			if e != nil {
				t.Fatal(e)
			}
			_, e = k.Create(ctx, "fixture/shared", doc)
			doc.Zero()
			if e != nil {
				t.Fatal(e)
			}
		}
		// Initial AppRole ttl=12s/max_ttl=36s. Cross the max lease to exercise
		// renewals followed by re-login under the same original service context.
		deadline := time.NewTimer(45 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		lastReadSucceeded := make([]bool, len(clients))
	loop:
		for {
			select {
			case <-ctx.Done():
				t.Fatal("authentication integration context expired")
			case <-deadline.C:
				break loop
			case <-tick.C:
				for i, c := range clients {
					k, e := c.KVv2("kv")
					if e != nil {
						t.Fatal(e)
					}
					expires := c.State().TokenExpiresAt
					r, e := k.ReadVersion(ctx, "fixture/shared", 1)
					if e != nil {
						// A bounded refresh backoff can cross the old token's expiry.
						// Only that fail-closed gap is allowed; other errors still fail.
						if !baoerr.IsCode(e, baoerr.CodeAuthenticationFailed) || expires == nil || time.Now().Before(*expires) {
							t.Fatal(e)
						}
						lastReadSucceeded[i] = false
						continue
					}
					lastReadSucceeded[i] = true
					var data map[string]any
					e = r.Data.Decode(&data)
					r.Data.Zero()
					if e != nil || data["owner"] != cluster.Spaces[i].Namespace {
						t.Fatal("namespace isolation", e)
					}
				}
			}
		}
		for i, p := range providers {
			if p.calls.Load() < 2 {
				t.Fatal("real AppRole re-login not observed")
			}
			if !lastReadSucceeded[i] {
				t.Fatal("read did not recover after AppRole re-login")
			}
		}
		// External atomic token file replacement uses two genuine restricted tokens.
		s := cluster.Spaces[0]
		dir := t.TempDir()
		path := filepath.Join(dir, "token")
		if e = os.WriteFile(path, s.Provision.Token, 0600); e != nil {
			t.Fatal(e)
		}
		provider, e := auth.NewTokenFile(path)
		if e != nil {
			t.Fatal(e)
		}
		c, e := bao.New(bao.Config{Address: cluster.Address, ClusterAlias: "isolated", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceNamed, Path: s.Namespace}, TLS: bao.TLSConfig{CAPEM: cluster.CAPEM}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}})
		if e != nil {
			t.Fatal(e)
		}
		if e = c.Start(ctx); e != nil {
			t.Fatal(e)
		}
		defer func() {
			x, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if e := c.Close(x); e != nil {
				t.Error(e)
			}
		}()
		before, e := provider.Snapshot(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path+".next", s.Replacement.Token, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.Rename(path+".next", path); e != nil {
			t.Fatal(e)
		}
		after, e := provider.Snapshot(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if before.Generation == after.Generation {
			t.Fatal("token file not refreshed")
		}
		before.Token.Zero()
		after.Token.Zero()
		ready, e := c.CheckReady(ctx, diagnostics.ReadProbe{Mount: "kv", Path: "fixture/shared", Version: 1})
		if e != nil || !ready.Ready {
			t.Fatal("replacement token", e)
		}
	})
}
