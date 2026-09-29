package bao

import (
	"context"
	"errors"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/kv"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitEmptyCAFileIsNotSystemTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-ca.pem")
	if e := os.WriteFile(path, nil, 0600); e != nil {
		t.Fatal(e)
	}
	cfg := testConfig()
	cfg.TLS.CAFile = path
	c, e := New(cfg)
	if c != nil {
		defer c.Close(context.Background())
	}
	if !baoerr.IsCode(e, baoerr.CodeInvalidArgument) {
		t.Fatal("explicit empty CA file silently fell back to system trust")
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
func TestPublicKeyDecodeAttemptsNotHTTPStatus(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		jsonData(w, map[string]any{"name": "key", "type": "ecdsa-p256", "latest_version": 1, "min_encryption_version": 0, "min_decryption_version": 1, "supports_signing": true, "supports_encryption": false, "exportable": false, "deletion_allowed": false, "derived": false, "keys": map[string]any{"1": map[string]any{"public_key": "broken"}}})
	}))
	tr, e := c.Transit("transit")
	if e != nil {
		t.Fatal(e)
	}
	_, e = tr.ReadPublicKey(context.Background(), "key", 1)
	var be *baoerr.Error
	if !errors.As(e, &be) || be.Attempts != 1 || be.HTTPStatus != 200 {
		t.Fatal("public key decode confused HTTP status with attempts")
	}
}
func TestAmbiguousWrite4xxRetainsUnknown(t *testing.T) {
	for _, status := range []int{400, 401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"proxy_result":"unspecified"}`))
			}))
			k, e := c.KVv2("kv")
			if e != nil {
				t.Fatal(e)
			}
			d, e := kv.NewDocument(map[string]any{"a": 1})
			if e != nil {
				t.Fatal(e)
			}
			defer d.Zero()
			if _, e = k.Create(context.Background(), "path", d); !baoerr.HasUnknownOutcome(e) || !baoerr.IsCode(e, baoerr.CodeInvalidResponse) {
				t.Fatal("unrecognized rejection envelope classified as definitely unexecuted")
			}
		})
	}
}
