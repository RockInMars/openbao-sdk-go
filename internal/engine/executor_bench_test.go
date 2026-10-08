package engine_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/testutil"
)

func BenchmarkExecutorConcurrent(b *testing.B) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer s.Close()
	pool := x509.NewCertPool()
	pool.AddCert(s.Certificate())
	tr, err := engine.NewTransport(engine.TransportConfig{TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}, MaxResponseBytes: 1024, DialTimeout: time.Second, TLSHandshakeTimeout: time.Second})
	if err != nil {
		b.Fatal(err)
	}
	defer tr.CloseIdleConnections()
	ex := engine.NewExecutor(testutil.NewHTTPSender(s.URL, "", engine.NewHTTPClient(tr), tr), engine.ExecutorConfig{RequestTimeout: 10 * time.Second, Concurrency: 8, MaxRequestBytes: 1024, MaxAttempts: 1})
	c := engine.Call{Operation: engine.KVReadVersion, Path: "/v1/kv/data/item", Credential: func(context.Context) (string, error) { return "fixture-token", nil }}
	perform := func() error {
		r, err := ex.Execute(context.Background(), c)
		if r != nil {
			r.Zero()
		}
		return err
	}
	b.Run("slots8", func(b *testing.B) {
		// This separate sample includes queuing and TLS/HTTP time. It is not
		// derived from ns/op, and runs outside the throughput measurement.
		const samples = 256
		latency := make([]int64, samples)
		var next atomic.Int64
		var failed atomic.Bool
		var wg sync.WaitGroup
		workers := runtime.GOMAXPROCS(0)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					index := int(next.Add(1) - 1)
					if index >= samples {
						return
					}
					started := time.Now()
					if perform() != nil {
						failed.Store(true)
					}
					latency[index] = time.Since(started).Nanoseconds()
				}
			}()
		}
		wg.Wait()
		if failed.Load() {
			b.Fatal("latency sample failed")
		}
		sort.Slice(latency, func(i, j int) bool { return latency[i] < latency[j] })
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if perform() != nil {
					failed.Store(true)
				}
			}
		})
		b.StopTimer()
		if failed.Load() {
			b.Fatal("executor operation failed")
		}
		b.ReportMetric(float64(latency[(samples*50+99)/100-1]), "p50-ns")
		b.ReportMetric(float64(latency[(samples*95+99)/100-1]), "p95-ns")
		b.ReportMetric(samples, "latency-samples")
		b.ReportMetric(float64(workers), "workers")
	})
}
