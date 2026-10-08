package bao

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

const futureDeletionTime = "9999-01-01T00:00:00Z"

func TestKVFutureDeletionStillReadable(t *testing.T) {
	for _, latest := range []bool{false, true} {
		name := "exact"
		if latest {
			name = "latest"
		}
		t.Run(name, func(t *testing.T) {
			var count atomic.Int32
			c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if (!latest && r.URL.Query().Get("version") != "2") || (latest && r.URL.Query().Get("version") != "") {
					t.Error("version selection changed")
				}
				body := strings.Replace(kvReadBody(2, `{"fixture":"scheduled"}`), `"deletion_time":""`, `"deletion_time":"`+futureDeletionTime+`"`, 1)
				_, _ = w.Write([]byte(body))
			}))
			k, err := c.KVv2("secret")
			if err != nil {
				t.Fatal(err)
			}
			if latest {
				result, err := k.ReadLatest(context.Background(), "scheduled")
				if err != nil || result == nil {
					t.Fatal("future scheduled deletion rejected a readable version")
				}
				defer result.Data.Zero()
				if result.Ref.Version != 2 {
					t.Fatal("response version lost")
				}
			} else {
				result, err := k.ReadVersion(context.Background(), "scheduled", 2)
				if err != nil || result == nil {
					t.Fatal("future scheduled deletion rejected a readable version")
				}
				defer result.Data.Zero()
				if result.Ref.Version != 2 {
					t.Fatal("response version changed")
				}
			}
			if count.Load() != 1 {
				t.Fatal("read unexpectedly replayed")
			}
		})
	}
}

func TestKVFutureDeletionDoesNotExplainMissingData(t *testing.T) {
	for _, status := range []int{200, 404} {
		c := startedClient(t, serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			body := strings.Replace(kvReadBody(2, `null`), `"deletion_time":""`, `"deletion_time":"`+futureDeletionTime+`"`, 1)
			_, _ = w.Write([]byte(body))
		}))
		k, err := c.KVv2("secret")
		if err != nil {
			t.Fatal(err)
		}
		result, err := k.ReadVersion(context.Background(), "scheduled", 2)
		want := baoerr.CodeInvalidResponse
		if status == 404 {
			want = baoerr.CodeNotFoundOrHidden
		}
		if result != nil || !baoerr.IsCode(err, want) {
			t.Fatal("future schedule incorrectly used as proof of already deleted data")
		}
	}
}
