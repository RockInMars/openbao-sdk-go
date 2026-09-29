package dependency_test

import (
	"context"
	api "github.com/openbao/openbao/api/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

// This test always imports the real pinned module. Contract-test mode never runs it.
func TestDependencyBaseline(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer s.Close()
	cfg := api.NewConfig()
	cfg.Address = s.URL
	cfg.HttpClient = s.Client()
	cfg.DisableEnvironment = true
	cfg.MaxRetries = 0
	cfg.DisableRedirects = true
	c, err := api.NewClient(cfg)
	if err != nil {
		t.Fatal("official client creation failed")
	}
	r, err := c.RawRequestWithContext(context.Background(), c.NewRequest("GET", "/v1/test"))
	if err != nil {
		t.Fatal("official request failed")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("official response status")
	}
}
