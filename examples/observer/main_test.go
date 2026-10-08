package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/observe"
)

func TestObserverUsesOnlyFiniteCounters(t *testing.T) {
	c := &counters{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.Observe(context.Background(), observe.Event{Operation: "KV_READ_VERSION", ClusterAlias: "secret-cluster", MountLabel: "secret-path", RequestID: "secret-request"})
			}
		}()
	}
	wg.Wait()
	c.Observe(context.Background(), observe.Event{Operation: "secret-operation", ErrorCode: "secret-error"})
	c.Observe(context.Background(), observe.Event{Operation: "KV_READ_VERSION", ErrorCode: baoerr.CodePermissionDenied})
	var output bytes.Buffer
	if err := c.writeTo(&output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "secret") || !strings.Contains(text, "operation=kv_read_version result=success count=1000") || !strings.Contains(text, "operation=kv_read_version result=permission_denied count=1") || !strings.Contains(text, "operation=other result=error count=1") {
		t.Fatalf("unbounded or incorrect output: %s", text)
	}
	if strings.Count(text, "\n") != 6 {
		t.Fatalf("unexpected label cardinality: %s", text)
	}
}
