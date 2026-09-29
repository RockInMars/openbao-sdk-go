package reader

import (
	"context"
	"encoding/pem"
	"fmt"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIndependentReader(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/data/owned/item" || r.URL.Query().Get("version") != "1" || r.Header.Get("X-Vault-Namespace") != "consumer" {
			t.Error("scope/version contract")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"request_id":"read-fixture","data":{"data":{"value":"fixture"},"metadata":{"version":1,"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}}`)
	}))
	defer srv.Close()
	token := sensitive.NewBytes([]byte("consumer-fixture-token"))
	defer token.Zero()
	provider, e := auth.NewStaticToken(token)
	if e != nil {
		t.Fatal(e)
	}
	c, e := bao.New(bao.Config{Address: srv.URL, ClusterAlias: "consumer", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceNamed, Path: "consumer"}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}, TLS: bao.TLSConfig{CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})}})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e = c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	defer func() {
		x, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if e := c.Close(x); e != nil {
			t.Error(e)
		}
	}()
	reader, e := New(c, "kv")
	if e != nil {
		t.Fatal(e)
	}
	value, e := reader.Read(ctx, "owned/item", 1)
	if e != nil {
		t.Fatal(e)
	}
	defer value.Data.Zero()
	if value.Ref.Version != 1 {
		t.Fatal("version not pinned")
	}
}
