package engine

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.example.com/infra/openbao-sdk-go/baoerr"
)

func TestRetryAfterLimitsAndCancellation(t *testing.T) {
	cfg := ExecutorConfig{BaseDelay: time.Millisecond, MaxDelay: 2 * time.Second}
	for _, s := range []string{"-1", "3", "nonsense", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)} {
		if _, ok := retryDelay(context.Background(), 8, cfg, http.Header{"Retry-After": []string{s}}); ok {
			t.Fatal("unbounded Retry-After accepted")
		}
	}
	for _, s := range []string{"0", "1", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)} {
		d, ok := retryDelay(context.Background(), 2, cfg, http.Header{"Retry-After": []string{s}})
		if !ok || d < 0 || d > 2*time.Second {
			t.Fatal("valid Retry-After rejected")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := wait(ctx, time.Second); !errors.Is(e, context.Canceled) {
		t.Fatal("wait ignored cancel")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	if _, ok := retryDelay(expired, 1, cfg, nil); ok {
		t.Fatal("expired budget extended")
	}
	if _, ok := retryDelay(context.Background(), 10, ExecutorConfig{BaseDelay: time.Second, MaxDelay: time.Millisecond}, nil); !ok {
		t.Fatal("capped delay lost")
	}
}
func TestDecodeAndIdentifierBoundaries(t *testing.T) {
	for _, b := range []string{"{}", "null", `{"data":null}`, `{"errors":"not-an-array","data":{}}`, `{"errors":["bad"],"data":{}}`} {
		if _, e := Data(&Response{Status: 200, Attempts: 1, Body: []byte(b)}, KVReadVersion); e == nil {
			t.Fatal("invalid data envelope accepted")
		}
	}
	if _, e := DecodeObject(nil, KVCreate); !baoerr.HasUnknownOutcome(e) {
		t.Fatal("nil successful write response not unknown")
	}
	for _, v := range []any{nil, "2", float64(2), json.Number("1.5"), json.Number("9223372036854775808")} {
		if _, ok := Int(v); ok {
			t.Fatal("noninteger accepted")
		}
	}
	for _, b := range []string{"bad", `{"request_id":12}`, `{"request_id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeeZ"}`, `{"request_id":"aaaaaaaa_bbbb-cccc-dddd-eeeeeeeeeeee"}`} {
		if requestID([]byte(b)) != "" {
			t.Fatal("untrusted identifier exposed")
		}
	}
	if requestID([]byte(`{"request_id":"ABCDEF12-aaaa-bbbb-cccc-123456789012"}`)) == "" {
		t.Fatal("safe identifier omitted")
	}
	if validToken("") || validToken("x\n") {
		t.Fatal("header injection accepted")
	}
}
func TestMalformedDeletedMetadata(t *testing.T) {
	for _, raw := range []string{`{}`, `{"data":null}`, `{"data":{"metadata":null}}`, `{"data":{"metadata":{"version":"1","destroyed":true}}}`, `{"data":{"metadata":{"version":0,"destroyed":true}}}`, `{"data":{"metadata":{"version":1,"deletion_time":"invalid"}}}`, `{"data":{"metadata":{"version":1,"deletion_time":"0001-01-01T00:00:00Z"}}}`} {
		var o map[string]any
		d := json.NewDecoder(strings.NewReader(raw))
		d.UseNumber()
		if e := d.Decode(&o); e != nil {
			t.Fatal(e)
		}
		if deletedMetadata(o) {
			t.Fatal("unproven version deletion asserted")
		}
	}
}
func TestTransportSafetyConstruction(t *testing.T) {
	for _, c := range []TransportConfig{{}, {TLS: &tls.Config{MinVersion: tls.VersionTLS10}, MaxResponseBytes: 10}, {TLS: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, MaxResponseBytes: 10}, {TLS: &tls.Config{MinVersion: tls.VersionTLS12}, MaxResponseBytes: 10, ProxyURL: "://bad"}} {
		if _, e := NewTransport(c); e == nil {
			t.Fatal("unsafe transport accepted")
		}
	}
	tr, e := NewTransport(TransportConfig{TLS: &tls.Config{MinVersion: tls.VersionTLS12}, MaxResponseBytes: 10, ProxyURL: "http://127.0.0.1:1234"})
	if e != nil {
		t.Fatal(e)
	}
	defer tr.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://127.0.0.1/", nil)
	if _, e = tr.RoundTrip(r); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled request dispatched")
	}
}

func TestAmbiguous404WritePreservesUnknown(t *testing.T) {
	d, _ := Lookup(KVCreate)
	e, _ := responseError(&Response{Status: 404, Attempts: 1, Body: []byte(`{}`)}, KVCreate, d)
	if !baoerr.IsCode(e, baoerr.CodeNotFoundOrHidden) || !baoerr.HasUnknownOutcome(e) {
		t.Fatal("ambiguous proxy 404 was declared nonexecuted")
	}
}

func Test404RejectionRequiresAPIProof(t *testing.T) {
	d, _ := Lookup(KVCreate)
	for _, tc := range []struct {
		body    string
		unknown bool
	}{
		{`{"errors":["path not found"]}`, false},
		{`{"errors":[]}`, true},
		{`{"errors":[""]}`, true},
		{`{"errors":[42]}`, true},
	} {
		e, retry := responseError(&Response{Status: 404, Attempts: 1, Body: []byte(tc.body)}, KVCreate, d)
		if !baoerr.IsCode(e, baoerr.CodeNotFoundOrHidden) || baoerr.HasUnknownOutcome(e) != tc.unknown || retry {
			t.Fatal("404 proof/effect contract changed")
		}
	}
}
