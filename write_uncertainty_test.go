package bao

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/kv"
)

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
func TestVoidErrorEnvelope(t *testing.T) {
	c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"errors":["denied"]}`)) }))
	k, _ := c.KVv2("secret")
	if e := k.DeleteVersions(context.Background(), "item", []int{1}); !baoerr.IsCode(e, baoerr.CodeInvalidResponse) || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("void write accepted error envelope as success")
	}
}

func TestAcceptedThenDisconnected(t *testing.T) {
	for _, op := range []engine.Operation{engine.PKIIssue, engine.KVCreate, engine.TransitSign} {
		t.Run(string(op), func(t *testing.T) {
			var writes atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				writes.Add(1)
				conn, _, e := w.(http.Hijacker).Hijack()
				if e != nil {
					t.Error("fixture hijack")
					return
				}
				conn.Close()
			}))
			e := c.execute(context.Background(), engine.Call{Operation: op, Path: "/v1/fixture/action", Payload: []byte(`{"value":"fixture"}`)}, "fixture", nil)
			if !baoerr.HasUnknownOutcome(e) || writes.Load() != 1 {
				t.Fatal("accepted write replayed or unknown lost")
			}
		})
	}
}
