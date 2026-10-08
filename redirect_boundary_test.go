package bao

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

func TestRedirectNeverLeaksToken(t *testing.T) {
	for _, status := range []int{307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetCalls, sourceCalls atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetCalls.Add(1)
				if r.Header.Get("X-Vault-Token") != "" {
					t.Error("token crossed redirect boundary")
				}
			}))
			defer target.Close()
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				sourceCalls.Add(1)
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
			}))
			k, _ := c.KVv2("secret")
			got, err := k.ReadVersion(context.Background(), "item", 1)
			if got != nil || !baoerr.IsCode(err, baoerr.CodeRedirectBlocked) || targetCalls.Load() != 0 || sourceCalls.Load() != 1 {
				t.Fatalf("redirect followed/retried: %v", err)
			}
		})
	}
}
