package bao

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// Health is intentionally usable before Start. Its admitted request must still
// belong to Client.Close, not only to a caller or a future Start context.
func TestCloseCancelsHealthBeforeStart(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	cfg := serverConfig(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	defer close(release)
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	caller, cancelCaller := context.WithCancel(context.Background())
	defer cancelCaller()
	done := make(chan error, 1)
	go func() { _, err := c.ClusterHealth(caller); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("health request never reached test server")
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelShutdown()
	if err = c.Close(shutdown); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Close did not report exhausted drain budget")
	}
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("Close did not cancel the admitted health request")
		}
	case <-time.After(time.Second):
		t.Fatal("pre-Start health request survived Close cancellation")
	}
	if err = c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
