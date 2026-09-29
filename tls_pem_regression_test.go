package bao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"git.example.com/infra/openbao-sdk-go/auth"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.example.com/infra/openbao-sdk-go/internal/testutil"
	"git.example.com/infra/openbao-sdk-go/sensitive"
)

func corruptPEMPrefix(kind string, valid []byte) []byte {
	return append([]byte("-----BEGIN "+kind+"-----\n!\n-----END "+kind+"-----\n"), valid...)
}

func TestTLSRejectsSkippedMalformedPEM(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	cfg := serverConfig(t, func(http.ResponseWriter, *http.Request) { t.Error("New performed network I/O") })
	for _, part := range []string{"ca", "client-cert", "client-key"} {
		t.Run(part, func(t *testing.T) {
			c := cfg
			c.TLS.CAPEM = bytes.Clone(cfg.TLS.CAPEM)
			c.TLS.ClientCertPEM = bytes.Clone(f.CertificatePEM)
			c.TLS.ClientKeyPEM = sensitive.NewBytes(f.PrivateKeyPEM)
			defer c.TLS.ClientKeyPEM.Zero()
			switch part {
			case "ca":
				c.TLS.CAPEM = corruptPEMPrefix("CERTIFICATE", c.TLS.CAPEM)
			case "client-cert":
				c.TLS.ClientCertPEM = corruptPEMPrefix("CERTIFICATE", c.TLS.ClientCertPEM)
			case "client-key":
				raw := corruptPEMPrefix("PRIVATE KEY", f.PrivateKeyPEM)
				c.TLS.ClientKeyPEM.Zero()
				c.TLS.ClientKeyPEM = sensitive.NewBytes(raw)
				defer c.TLS.ClientKeyPEM.Zero()
				clear(raw)
			}
			tr, err := transportFor(c)
			if tr != nil {
				tr.CloseIdleConnections()
			}
			if err == nil {
				t.Fatal("TLS accepted material after skipping a malformed PEM block")
			}
		})
	}
}

func TestTLSStrictPEMPreservesMutualTLS(t *testing.T) {
	f := testutil.NewPKIFixture(t)
	roots := x509.NewCertPool()
	roots.AddCert(f.CA)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || !bytes.Equal(r.TLS.PeerCertificates[0].Raw, f.Leaf.Raw) {
			t.Error("mutual TLS identity was not verified")
		}
		_, _ = w.Write([]byte(`{"initialized":true,"sealed":false,"standby":false}`))
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	srv.StartTLS()
	defer srv.Close()
	ecDER, err := x509.MarshalECPrivateKey(f.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(ecDER)
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecDER})
	defer clear(ecPEM)
	for _, key := range [][]byte{f.PrivateKeyPEM, ecPEM, bytes.ReplaceAll(f.PrivateKeyPEM, []byte("\n"), []byte("\r\n"))} {
		token, err := auth.NewStaticToken(sensitive.NewBytes([]byte("fixture-token")))
		if err != nil {
			t.Fatal(err)
		}
		keyMaterial := sensitive.NewBytes(key)
		defer keyMaterial.Zero()
		cfg := Config{Address: srv.URL, ClusterAlias: "fixture", Namespace: NamespaceConfig{Mode: NamespaceRoot}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: token}, TLS: TLSConfig{CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), ClientCertPEM: f.CertificatePEM, ClientKeyPEM: keyMaterial}}
		c := startedClient(t, cfg)
		result, err := c.ClusterHealth(context.Background())
		if err != nil || result == nil || !result.Initialized {
			t.Fatal("strict PEM rejected a supported mTLS identity")
		}
	}
}
