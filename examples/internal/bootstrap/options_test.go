package bootstrap

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/observe"
)

type countObserver struct{ calls atomic.Int64 }

func (o *countObserver) Observe(context.Context, observe.Event) { o.calls.Add(1) }

func TestRunForwardsOptionsAndKeepsOldCalls(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/data/item" || r.URL.Query().Get("version") != "1" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"fixture-only"},"metadata":{"version":1,"created_time":"2026-01-01T00:00:00Z","deletion_time":"","destroyed":false}}}`))
	}))
	defer s.Close()
	dir := t.TempDir()
	ca, token := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "token")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(token, []byte("fixture-token"), 0600); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"SDK_BAO_ADDRESS": s.URL, "SDK_BAO_CLUSTER": "fixture", "SDK_BAO_NAMESPACE_MODE": "root", "SDK_BAO_NAMESPACE": "", "SDK_BAO_TOKEN_FILE": token, "SDK_BAO_CA_FILE": ca} {
		t.Setenv(k, v)
	}
	action := func(ctx context.Context, c *bao.Client) error {
		k, err := c.KVv2("kv")
		if err != nil {
			return err
		}
		r, err := k.ReadVersion(ctx, "item", 1)
		if err == nil {
			r.Data.Zero()
		}
		return err
	}
	o := &countObserver{}
	if err := Run(action, bao.WithObserver(o)); err != nil {
		t.Fatal(err)
	}
	if o.calls.Load() != 1 {
		t.Fatalf("option not forwarded: %d", o.calls.Load())
	}
	if err := Run(action); err != nil {
		t.Fatal(err)
	}
}
