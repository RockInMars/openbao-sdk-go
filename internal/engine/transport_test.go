package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransportDecodingBoundsAndTruncation(t *testing.T) {
	var compressed bytes.Buffer
	g := gzip.NewWriter(&compressed)
	g.Write([]byte(strings.Repeat("x", 64)))
	g.Close()
	for _, tc := range []struct {
		name, encoding string
		body           []byte
		limit          int64
		want           error
		truncated      bool
	}{
		{"gzip", "gzip", compressed.Bytes(), 128, nil, false}, {"decoded bound", "gzip", compressed.Bytes(), 16, ErrResponseTooLarge, false},
		{"unsupported", "br", []byte("secret"), 128, ErrInvalidResponse, false}, {"bad gzip", "gzip", []byte("secret"), 128, ErrInvalidResponse, false},
		{"truncated", "", []byte("short"), 128, io.ErrUnexpectedEOF, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", tc.encoding)
				if tc.truncated {
					w.Header().Set("Content-Length", "999")
				}
				w.Write(tc.body)
			}))
			defer s.Close()
			roots := x509.NewCertPool()
			roots.AddCert(s.Certificate())
			tr, err := NewTransport(TransportConfig{TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, MaxResponseBytes: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			defer tr.CloseIdleConnections()
			ctx, exchange := NewExchange(context.Background())
			req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			response, err := tr.RoundTrip(req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v got %v", tc.want, err)
			}
			snapshot, captured, dispatched := exchange.Snapshot()
			if !dispatched || !errors.Is(captured, tc.want) || snapshot == nil {
				t.Fatal("exchange lost bounded outcome")
			}
			if tc.want != nil {
				if len(snapshot.Body) != 0 {
					t.Fatal("failed response retained body")
				}
				return
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if string(body) != strings.Repeat("x", 64) || !response.Uncompressed || response.Header.Get("Content-Encoding") != "" {
				t.Fatal("gzip was not normalized")
			}
		})
	}
}

func TestLoopbackRequiresAnIPAddress(t *testing.T) {
	for host, want := range map[string]bool{"127.0.0.1": true, "::1": true, "localhost": false, "127.0.0.1.example.org": false, "192.0.2.1": false} {
		if IsLoopbackHost(host) != want {
			t.Fatal(host)
		}
	}
}
